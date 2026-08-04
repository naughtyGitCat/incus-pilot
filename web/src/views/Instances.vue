<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Instances</h2>
      <n-space>
        <n-button type="primary" @click="fetchInstances">Refresh</n-button>
        <n-button type="success" @click="showCreateModal = true">Create Instance</n-button>
      </n-space>
    </n-space>

    <n-data-table :columns="columns" :data="instances" :loading="loading" />

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
const instances = ref([])
const showTerminal = ref(false)
const terminalContainer = ref<HTMLElement | null>(null)

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

const openTerminal = (name: string) => {
  showTerminal.value = true
  message.info(`Opening terminal for ${name}`)
}

onMounted(fetchInstances)
</script>

<style scoped>
.layout {
  padding: 24px;
}
</style>
