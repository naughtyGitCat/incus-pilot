<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Local Images</h2>
      <n-space>
        <n-button type="primary" size="small" @click="fetchImages">Refresh</n-button>
        <n-button type="success" size="small" @click="showDownloadModal = true">Download Image</n-button>
      </n-space>
    </n-space>

    <n-data-table :columns="columns" :data="images" :loading="loading" />

    <!-- 下载/拉取新镜像 Modal -->
    <n-modal v-model:show="showDownloadModal" preset="card" title="Pull Remote Image to Local" style="width: 500px">
      <n-form :model="downloadForm" label-placement="left" label-width="120">
        <n-form-item label="Image Server">
          <n-select v-model:value="downloadForm.server" :options="serverOptions" />
        </n-form-item>
        <n-form-item label="OS Alias">
          <n-select v-model:value="downloadForm.alias" :options="aliasOptions" filterable tag placeholder="Select or type custom alias" />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showDownloadModal = false">Cancel</n-button>
          <n-button type="primary" :loading="downloading" @click="handleDownload">Download</n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NButton, NTag, NSpace, useMessage } from 'naive-ui'
import api from '../api'

const message = useMessage()
const loading = ref(false)
const downloading = ref(false)
const images = ref([])
const showDownloadModal = ref(false)

const downloadForm = ref({
  server: 'https://images.linuxcontainers.org',
  alias: 'rockylinux/9'
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

const formatSize = (bytes: number) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const columns = [
  {
    title: 'Aliases',
    key: 'aliases',
    render(row: any) {
      const aliases = row.aliases || []
      if (!aliases.length) return '-'
      return aliases.map((a: any) => a.name).join(', ')
    }
  },
  {
    title: 'Architecture',
    key: 'architecture'
  },
  {
    title: 'Type',
    key: 'type',
    render(row: any) {
      return h(NTag, { type: 'info' }, { default: () => row.type || 'container' })
    }
  },
  {
    title: 'Size',
    key: 'size',
    render(row: any) {
      return formatSize(row.size)
    }
  },
  {
    title: 'Fingerprint',
    key: 'fingerprint',
    render(row: any) {
      return row.fingerprint ? row.fingerprint.substring(0, 12) + '...' : '-'
    }
  },
  {
    title: 'Actions',
    key: 'actions',
    render(row: any) {
      return h(
        NButton,
        {
          size: 'small',
          type: 'error',
          onClick: () => deleteImage(row.fingerprint)
        },
        { default: () => 'Delete' }
      )
    }
  }
]

const fetchImages = async () => {
  loading.value = true
  try {
    const res = await api.get('/incus/images?recursion=1')
    images.value = res.data.metadata || []
  } catch (err) {
    message.error('Failed to fetch local images')
  } finally {
    loading.value = false
  }
}

const handleDownload = async () => {
  downloading.value = true
  try {
    const payload = {
      source: {
        type: 'image',
        mode: 'pull',
        server: downloadForm.value.server,
        alias: downloadForm.value.alias
      }
    }
    await api.post('/incus/images', payload)
    message.success(`Started pulling image ${downloadForm.value.alias}`)
    showDownloadModal.value = false
    setTimeout(fetchImages, 4000)
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to pull image')
  } finally {
    downloading.value = false
  }
}

const deleteImage = async (fingerprint: string) => {
  try {
    await api.delete(`/incus/images/${fingerprint}`)
    message.success('Image deleted')
    fetchImages()
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Failed to delete image')
  }
}

onMounted(fetchImages)
</script>

<style scoped>
.layout {
  padding: 0;
}
</style>
