<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Instances</h2>
      <n-space>
        <n-button type="primary" size="small" @click="fetchInstances">Refresh</n-button>
        <n-button type="success" size="small" @click="showCreateModal = true">Create Instance</n-button>
      </n-space>
    </n-space>

    <n-data-table :columns="columns" :data="instances" :loading="loading" />

    <!-- 创建容器 Modal -->
    <n-modal v-model:show="showCreateModal" preset="card" title="Create New Instance" style="width: 500px">
      <n-form :model="createForm" label-placement="left" label-width="120">
        <n-form-item label="Name">
          <n-input v-model:value="createForm.name" placeholder="my-container" />
        </n-form-item>
        <n-form-item label="Image Server">
          <n-select v-model:value="createForm.server" :options="serverOptions" />
        </n-form-item>
        <n-form-item label="OS Alias">
          <n-select v-model:value="createForm.alias" :options="aliasOptions" filterable tag placeholder="Select or type custom alias" />
        </n-form-item>
        <n-form-item label="Type">
          <n-radio-group v-model:value="createForm.type">
            <n-radio value="container">Container</n-radio>
            <n-radio value="virtual-machine">Virtual Machine</n-radio>
          </n-radio-group>
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCreateModal = false">Cancel</n-button>
          <n-button type="primary" :loading="creating" @click="handleCreate">Create</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 异步操作/创建进度 Modal -->
    <n-modal v-model:show="showProgressModal" preset="card" title="Operation Progress" style="width: 450px" :closable="false">
      <n-space vertical align="center" style="width: 100%; padding: 12px 0">
        <n-spin size="large" />
        <div style="font-weight: bold; margin-top: 12px">{{ progressStatus }}</div>
        <n-progress type="line" :percentage="progressPercentage" :indicator-placement="'inside'" style="width: 100%; margin-top: 12px" />
        <div style="font-size: 12px; color: #888">{{ progressMessage }}</div>
      </n-space>
    </n-modal>

    <!-- 终端 Modal -->
    <n-modal v-model:show="showTerminal" preset="card" title="Web Terminal" style="width: 800px">
      <div ref="terminalContainer" style="height: 400px; background: #000"></div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import api from '../api'

const message = useMessage()
const loading = ref(false)
const creating = ref(false)
const instances = ref([])
const showCreateModal = ref(false)
const showProgressModal = ref(false)
const progressStatus = ref('Initializing...')
const progressPercentage = ref(0)
const progressMessage = ref('Connecting to Incus operation stream...')

const showTerminal = ref(false)
const terminalContainer = ref<HTMLElement | null>(null)

const createForm = ref({
  name: '',
  server: 'https://images.linuxcontainers.org',
  alias: 'rockylinux/9',
  type: 'container'
})

const serverOptions = [
  { label: 'Linux Containers (images:)', value: 'https://images.linuxcontainers.org' }
]

const aliasOptions = [
  { label: 'Rocky Linux 9', value: 'rockylinux/9' },
  { label: 'Rocky Linux 8', value: 'rockylinux/8' },
  { label: 'Debian 12 (Bookworm)', value: 'debian/12' },
  { label: 'Ubuntu 24.04 LTS (Noble)', value: 'ubuntu/24.04' },
  { label: 'Alpine 3.20', value: 'alpine/3.20' },
  { label: 'Fedora 40', value: 'fedora/40' },
  { label: 'AlmaLinux 9', value: 'almalinux/9' },
  { label: 'CentOS Stream 9', value: 'centos/9-Stream' },
  { label: 'Arch Linux', value: 'archlinux' }
]

const columns = [
  { title: 'Name', key: 'name' },
  {
    title: 'Status',
    key: 'status',
    render(row: any) {
      const type = row.status === 'Running' ? 'success' : 'error'
      return h(NTag, { type }, { default: () => row.status })
    }
  },
  { title: 'Type', key: 'type' },
  {
    title: 'IPv4',
    key: 'state',
    render(row: any) {
      const eth0 = row.state?.network?.eth0 || row.state?.network?.incusbr0
      const addresses = eth0?.addresses || []
      const ipv4 = addresses.find((a: any) => a.family === 'inet')
      return ipv4 ? ipv4.address : '-'
    }
  },
  {
    title: 'Actions',
    key: 'actions',
    render(row: any) {
      return h(NSpace, {}, {
        default: () => [
          h(
            NButton,
            {
              size: 'small',
              type: row.status === 'Running' ? 'warning' : 'success',
              onClick: () => toggleState(row)
            },
            { default: () => (row.status === 'Running' ? 'Stop' : 'Start') }
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'info',
              disabled: row.status !== 'Running',
              onClick: () => openTerminal(row.name)
            },
            { default: () => 'Terminal' }
          ),
          h(
            NButton,
            {
              size: 'small',
              type: 'error',
              onClick: () => deleteInstance(row.name)
            },
            { default: () => 'Delete' }
          )
        ]
      })
    }
  }
]

const fetchInstances = async () => {
  loading.value = true
  try {
    const res = await api.get('/incus/instances?recursion=1')
    instances.value = res.data.metadata || []
  } catch (err: any) {
    message.error('Failed to fetch instances')
  } finally {
    loading.value = false
  }
}

const subscribeOperationEvents = (operationId: string) => {
  showProgressModal.value = true
  progressStatus.value = 'Creating Instance'
  progressPercentage.value = 15
  progressMessage.value = 'Downloading image & creating instance...'

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token') || ''
  const wsUrl = `${protocol}//${window.location.host}/api/events?token=${token}`

  const ws = new WebSocket(wsUrl)

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.type === 'operation' && data.metadata) {
        const op = data.metadata
        if (op.id === operationId || op.resources?.instances?.some((i: string) => i.includes(createForm.value.name))) {
          progressStatus.value = op.description || 'Creating Instance'
          if (op.status === 'Success') {
            progressPercentage.value = 100
            progressMessage.value = 'Completed!'
            message.success(`Instance ${createForm.value.name} created successfully!`)
            setTimeout(() => {
              showProgressModal.value = false
              fetchInstances()
              ws.close()
            }, 1000)
          } else if (op.status === 'Failure') {
            message.error(op.err || 'Instance creation failed')
            showProgressModal.value = false
            ws.close()
          } else {
            if (progressPercentage.value < 90) progressPercentage.value += 15
            if (op.metadata?.download_progress) {
              progressMessage.value = `Downloading: ${op.metadata.download_progress}`
            }
          }
        }
      }
    } catch (e) {}
  }

  ws.onerror = () => {
    pollOperationStatus(operationId)
  }
}

const pollOperationStatus = (operationId: string) => {
  const interval = setInterval(async () => {
    try {
      const res = await api.get(`/incus/operations/${operationId}`)
      const op = res.data.metadata
      if (op.status === 'Success') {
        clearInterval(interval)
        progressPercentage.value = 100
        showProgressModal.value = false
        message.success('Creation completed!')
        fetchInstances()
      } else if (op.status === 'Failure') {
        clearInterval(interval)
        showProgressModal.value = false
        message.error(op.err || 'Creation failed')
      }
    } catch (e) {
      clearInterval(interval)
      showProgressModal.value = false
      fetchInstances()
    }
  }, 2000)
}

const handleCreate = async () => {
  if (!createForm.value.name) {
    message.warning('Please enter an instance name')
    return
  }
  creating.value = true
  try {
    const payload = {
      name: createForm.value.name,
      type: createForm.value.type,
      source: {
        type: 'image',
        mode: 'pull',
        server: createForm.value.server,
        alias: createForm.value.alias,
        protocol: 'simplestreams',
        instance_type: createForm.value.type
      }
    }
    const res = await api.post('/incus/instances', payload)
    showCreateModal.value = false
    const opUrl = res.data.operation || ''
    const opId = opUrl.split('/').pop() || ''

    subscribeOperationEvents(opId)
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to create instance')
  } finally {
    creating.value = false
  }
}

const toggleState = async (row: any) => {
  const action = row.status === 'Running' ? 'stop' : 'start'
  try {
    await api.put(`/incus/instances/${row.name}/state`, { action })
    message.success(`Action ${action} initiated`)
    setTimeout(fetchInstances, 2000)
  } catch (err: any) {
    message.error(`Failed to ${action} instance`)
  }
}

const deleteInstance = async (name: string) => {
  try {
    await api.delete(`/incus/instances/${name}`)
    message.success(`Instance ${name} deleted`)
    fetchInstances()
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to delete instance')
  }
}

const openTerminal = (name: string) => {
  showTerminal.value = true
  message.info(`Opening terminal for ${name}`)
}

onMounted(fetchInstances)
</script>

<style scoped>
.layout {
  padding: 0;
}
</style>
