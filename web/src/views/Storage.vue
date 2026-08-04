<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 16px">
      <h2>Storage Pools</h2>
      <n-button type="primary" @click="fetchStorage">Refresh</n-button>
    </n-space>

    <n-data-table :columns="columns" :data="pools" :loading="loading" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useMessage } from 'naive-ui'
import api from '../api'

const message = useMessage()
const loading = ref(false)
const pools = ref([])

const columns = [
  { title: 'Name', key: 'name' },
  { title: 'Driver', key: 'driver' },
  { title: 'Status', key: 'status' },
  { title: 'Source', key: 'config.source' }
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
  padding: 24px;
}
</style>
