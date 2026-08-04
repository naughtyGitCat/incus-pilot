<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Storage Pools</h2>
      <n-button type="primary" size="small" @click="fetchStorage">Refresh</n-button>
    </n-space>

    <n-data-table :columns="columns" :data="pools" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import { NTag, useMessage } from 'naive-ui'
import api from '../api'

const message = useMessage()
const loading = ref(false)
const pools = ref([])

const columns = [
  { title: 'Pool Name', key: 'name' },
  {
    title: 'Driver',
    key: 'driver',
    render(row: any) {
      return h(NTag, { type: 'info' }, { default: () => row.driver })
    }
  },
  {
    title: 'Status',
    key: 'status',
    render(row: any) {
      return h(NTag, { type: 'success' }, { default: () => row.status || 'Created' })
    }
  },
  {
    title: 'Source / Location',
    key: 'config',
    render(row: any) {
      return row.config?.source || row.config?.['btrfs.pool_name'] || '-'
    }
  },
  {
    title: 'Used By (Count)',
    key: 'used_by',
    render(row: any) {
      return (row.used_by || []).length
    }
  }
]

const fetchStorage = async () => {
  loading.value = true
  try {
    const res = await api.get('/incus/storage-pools?recursion=1')
    pools.value = res.data.metadata || []
  } catch (err) {
    message.error('Failed to fetch storage pools')
  } finally {
    loading.value = false
  }
}

onMounted(fetchStorage)
</script>

<style scoped>
.layout {
  padding: 0;
}
</style>
