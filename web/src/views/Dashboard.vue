<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 24px">
      <h2>🚀 Incus Pilot Overview</h2>
      <n-button @click="handleLogout">Logout</n-button>
    </n-space>

    <n-grid :cols="4" :x-gap="12">
      <n-gi>
        <n-card title="Total Instances">
          <div class="stat-value">{{ instancesCount }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card title="Running">
          <div class="stat-value text-success">{{ runningCount }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card title="Stopped">
          <div class="stat-value text-warning">{{ stoppedCount }}</div>
        </n-card>
      </n-gi>
      <n-gi>
        <n-card title="Incus Version">
          <div class="stat-value">{{ serverVersion }}</div>
        </n-card>
      </n-gi>
    </n-grid>

    <div style="margin-top: 24px">
      <Instances />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import Instances from './Instances.vue'
import api from '../api'

const router = useRouter()
const instancesCount = ref(0)
const runningCount = ref(0)
const stoppedCount = ref(0)
const serverVersion = ref('v6.23')

const fetchOverview = async () => {
  try {
    const res = await api.get('/incus/instances?recursion=1')
    const list = res.data.metadata || []
    instancesCount.value = list.length
    runningCount.value = list.filter((i: any) => i.status === 'Running').length
    stoppedCount.value = list.filter((i: any) => i.status !== 'Running').length
  } catch (err) {}
}

const handleLogout = () => {
  localStorage.removeItem('token')
  router.push('/login')
}

onMounted(fetchOverview)
</script>

<style scoped>
.layout {
  padding: 24px;
}
.stat-value {
  font-size: 28px;
  font-weight: bold;
}
.text-success {
  color: #63e2b7;
}
.text-warning {
  color: #f2c97d;
}
</style>
