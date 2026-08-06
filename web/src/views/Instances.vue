<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Instances</h2>
      <n-space>
        <n-button type="primary" size="small" @click="fetchInstances">Refresh</n-button>
        <n-button type="success" size="small" @click="showCreateModal = true">Create Instance</n-button>
      </n-space>
    </n-space>

    <n-data-table :columns="columns" :data="instances" :loading="loading" :scroll-x="900" />

    <!-- 创建容器 Modal -->
    <n-modal v-model:show="showCreateModal" preset="card" title="Create New Instance" style="width: 90%; max-width: 550px">
      <n-form :model="createForm" label-placement="left" label-width="140">
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
        <n-form-item label="Inject SSH Key">
          <n-switch v-model:value="createForm.enableSSH" />
        </n-form-item>
        <template v-if="createForm.enableSSH">
          <n-form-item label="Select Saved Key">
            <n-select v-model:value="selectedKeyName" :options="savedKeyOptions" @update:value="onKeySelect" placeholder="Select a saved key" />
          </n-form-item>
          <n-form-item label="SSH Public Key">
            <n-input
              v-model:value="createForm.sshKey"
              type="textarea"
              :rows="3"
              placeholder="ssh-rsa AAAAB3NzaC1..."
            />
          </n-form-item>
          <n-space justify="end" style="margin-bottom: 12px">
            <n-button size="small" type="info" @click="showKeyManageModal = true">Manage Saved Keys</n-button>
          </n-space>
        </template>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showCreateModal = false">Cancel</n-button>
          <n-button type="primary" :loading="creating" @click="handleCreate">Create</n-button>
        </n-space>
      </template>
    </n-modal>

    <!-- 管理保存的 SSH 公钥 Modal -->
    <n-modal v-model:show="showKeyManageModal" preset="card" title="Manage SSH Public Keys" style="width: 90%; max-width: 550px">
      <n-space vertical style="width: 100%">
        <n-space align="center">
          <n-input v-model:value="newKeyForm.name" placeholder="Name" style="width: 120px" />
          <n-input v-model:value="newKeyForm.key" placeholder="ssh-rsa AAA..." style="width: 250px" />
          <n-button type="primary" size="small" @click="saveNewKey">Add Key</n-button>
        </n-space>
        <n-data-table :columns="keyTableColumns" :data="savedKeys" size="small" />
      </n-space>
    </n-modal>

    <!-- 异步操作/创建进度 Modal -->
    <n-modal v-model:show="showProgressModal" preset="card" title="Operation Progress" style="width: 90%; max-width: 450px" :closable="false">
      <n-space vertical align="center" style="width: 100%; padding: 12px 0">
        <n-spin size="large" />
        <div style="font-weight: bold; margin-top: 12px">{{ progressStatus }}</div>
        <n-progress type="line" :percentage="progressPercentage" :indicator-placement="'inside'" style="width: 100%; margin-top: 12px" />
        <div style="font-size: 12px; color: #888">{{ progressMessage }}</div>
      </n-space>
    </n-modal>

    <!-- Web 终端 Modal -->
    <n-modal
      v-model:show="showTerminal"
      preset="card"
      :title="`Web Terminal - ${currentTerminalInstance}`"
      style="width: 95%; max-width: 900px"
      :on-after-enter="initTerminal"
      :on-after-leave="closeTerminal"
    >
      <div ref="terminalContainer" style="height: 60vh; max-height: 500px; min-height: 300px; background: #1e1e1e; padding: 4px; border-radius: 4px"></div>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed, nextTick, h } from 'vue'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import { Terminal } from 'xterm'
import { FitAddon } from 'xterm-addon-fit'
import api from '../api'

const message = useMessage()
const loading = ref(false)
const creating = ref(false)
const instances = ref([])
const showCreateModal = ref(false)
const showProgressModal = ref(false)
const showKeyManageModal = ref(false)

const progressStatus = ref('Initializing...')
const progressPercentage = ref(0)
const progressMessage = ref('Connecting to Incus operation stream...')

const showTerminal = ref(false)
const currentTerminalInstance = ref('')
const terminalContainer = ref<HTMLElement | null>(null)
let term: Terminal | null = null
let fitAddon: FitAddon | null = null
let ws: WebSocket | null = null

const defaultSSHKey = 'ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC...'

const savedKeys = ref<{ name: string; key: string }[]>([])
const selectedKeyName = ref('Example Key')

const loadSavedKeys = () => {
  const stored = localStorage.getItem('incus_pilot_ssh_keys')
  if (stored) {
    try {
      savedKeys.value = JSON.parse(stored)
    } catch (e) {
      savedKeys.value = [{ name: 'Example Key', key: defaultSSHKey }]
    }
  } else {
    savedKeys.value = [{ name: 'Example Key', key: defaultSSHKey }]
    localStorage.setItem('incus_pilot_ssh_keys', JSON.stringify(savedKeys.value))
  }
}

const savedKeyOptions = computed(() => {
  return savedKeys.value.map(k => ({ label: k.name, value: k.name }))
})

const onKeySelect = (val: string) => {
  const item = savedKeys.value.find(k => k.name === val)
  if (item) {
    createForm.value.sshKey = item.key
  }
}

const newKeyForm = ref({ name: '', key: '' })

const saveNewKey = () => {
  if (!newKeyForm.value.name || !newKeyForm.value.key) {
    message.warning('Please enter key name and key content')
    return
  }
  savedKeys.value.push({ name: newKeyForm.value.name, key: newKeyForm.value.key })
  localStorage.setItem('incus_pilot_ssh_keys', JSON.stringify(savedKeys.value))
  message.success('SSH Key added')
  newKeyForm.value.name = ''
  newKeyForm.value.key = ''
}

const deleteSavedKey = (name: string) => {
  savedKeys.value = savedKeys.value.filter(k => k.name !== name)
  localStorage.setItem('incus_pilot_ssh_keys', JSON.stringify(savedKeys.value))
  message.success('Key removed')
}

const keyTableColumns = [
  { title: 'Name', key: 'name' },
  {
    title: 'Key Preview',
    key: 'key',
    render(row: any) {
      return row.key ? row.key.substring(0, 24) + '...' : '-'
    }
  },
  {
    title: 'Action',
    key: 'action',
    render(row: any) {
      return h(
        NButton,
        {
          size: 'small',
          type: 'error',
          onClick: () => deleteSavedKey(row.name)
        },
        { default: () => 'Delete' }
      )
    }
  }
]

const createForm = ref({
  name: '',
  server: 'https://images.linuxcontainers.org',
  alias: 'alpine/3.21',
  type: 'container',
  enableSSH: true,
  sshKey: defaultSSHKey
})

const serverOptions = [
  { label: 'Linux Containers (images:)', value: 'https://images.linuxcontainers.org' }
]

const aliasOptions = [
  { label: 'Alpine 3.21', value: 'alpine/3.21' },
  { label: 'Rocky Linux 9', value: 'rockylinux/9' },
  { label: 'Rocky Linux 8', value: 'rockylinux/8' },
  { label: 'Debian 12 (Bookworm)', value: 'debian/12' },
  { label: 'Ubuntu 24.04 LTS (Noble)', value: 'ubuntu/24.04' },
  { label: 'Fedora 40', value: 'fedora/40' },
  { label: 'AlmaLinux 9', value: 'almalinux/9' },
  { label: 'CentOS Stream 9', value: 'centos/9-Stream' },
  { label: 'Arch Linux', value: 'archlinux' }
]

const formatSize = (bytes: number) => {
  if (!bytes || bytes <= 0) return '-'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

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
    title: 'Resources (CPU / Mem / Disk)',
    key: 'resources',
    render(row: any) {
      const cpuLimit = row.config?.['limits.cpu'] ? `${row.config['limits.cpu']} Core` : 'Shared CPU'
      const memLimit = row.config?.['limits.memory'] || ''
      const memUsage = row.state?.memory?.usage ? formatSize(row.state.memory.usage) : ''
      const memText = memUsage ? (memLimit ? `${memUsage} / ${memLimit}` : memUsage) : (memLimit || 'Unlimited')

      const diskLimit = row.expanded_devices?.root?.size || row.config?.['limits.disk'] || ''
      const diskUsage = row.state?.disk?.root?.usage ? formatSize(row.state.disk.root.usage) : ''
      const diskText = diskUsage ? (diskLimit ? `${diskUsage} / ${diskLimit}` : diskUsage) : (diskLimit || 'Default Pool')

      return h('div', { style: { fontSize: '12px' } }, [
        h('div', {}, `CPU: ${cpuLimit}`),
        h('div', {}, `Mem: ${memText}`),
        h('div', {}, `Disk: ${diskText}`)
      ])
    }
  },
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

  const eventWs = new WebSocket(wsUrl)

  eventWs.onmessage = (event) => {
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
              eventWs.close()
            }, 1000)
          } else if (op.status === 'Failure') {
            message.error(op.err || 'Instance creation failed')
            showProgressModal.value = false
            eventWs.close()
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

  eventWs.onerror = () => {
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
    const config: Record<string, string> = {}
    if (createForm.value.enableSSH && createForm.value.sshKey) {
      const cleanKey = createForm.value.sshKey.trim()
      config['user.user-data'] = `#cloud-config
package_update: true
packages:
  - openssh-server
  - openssh
ssh_authorized_keys:
  - "${cleanKey}"
runcmd:
  - [ sh, -c, "apk add --no-cache openssh-server openssh || dnf install -y openssh-server || apt-get update && apt-get install -y openssh-server || true" ]
  - [ sh, -c, "mkdir -p /root/.ssh && chmod 700 /root/.ssh" ]
  - [ sh, -c, "echo '${cleanKey}' >> /root/.ssh/authorized_keys && chmod 600 /root/.ssh/authorized_keys" ]
  - [ sh, -c, "ssh-keygen -A 2>/dev/null || true" ]
  - [ sh, -c, "sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config || true" ]
  - [ sh, -c, "systemctl enable --now sshd || systemctl enable --now ssh || service ssh restart || /usr/sbin/sshd || true" ]
`
    }

    const payload = {
      name: createForm.value.name,
      type: createForm.value.type,
      config: config,
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
  currentTerminalInstance.value = name
  showTerminal.value = true
}

const initTerminal = () => {
  if (!terminalContainer.value) return
  if (term) closeTerminal()

  term = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#ffffff'
    }
  })

  fitAddon = new FitAddon()
  term.loadAddon(fitAddon)

  term.open(terminalContainer.value)
  fitAddon.fit()
  term.focus()

  const name = currentTerminalInstance.value
  term.writeln(`Connecting to ${name}...`)

  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  const token = localStorage.getItem('token') || ''
  const cols = term.cols || 80
  const rows = term.rows || 24
  const wsUrl = `${protocol}//${window.location.host}/api/ws/exec/${name}?token=${token}&cols=${cols}&rows=${rows}`

  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    term?.clear()
    term?.focus()
  }

  ws.onmessage = (event) => {
    term?.write(event.data)
  }

  term.onData((data) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(data)
    }
  })

  ws.onclose = () => {
    term?.writeln('\r\n\x1b[31m[Session closed]\x1b[0m')
  }

  ws.onerror = () => {
    term?.writeln('\r\n\x1b[31m[Connection error]\x1b[0m')
  }
}

const closeTerminal = () => {
  if (ws) {
    ws.close()
    ws = null
  }
  if (term) {
    term.dispose()
    term = null
  }
  fitAddon = null
}

onMounted(() => {
  loadSavedKeys()
  fetchInstances()
})
</script>

<style scoped>
.layout {
  padding: 0;
}
</style>
