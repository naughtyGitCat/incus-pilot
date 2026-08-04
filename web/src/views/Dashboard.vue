<template>
  <div class="layout">
    <n-space justify="space-between" align="center" style="margin-bottom: 24px">
      <h2>🚀 Incus Pilot Overview</h2>
      <n-button type="warning" size="small" @click="handleLogout">Logout</n-button>
    </n-space>

    <!-- 概览数据卡片 -->
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
        <n-card title="Storage Pools">
          <div class="stat-value text-info">{{ storagePoolsCount }}</div>
        </n-card>
      </n-gi>
    </n-grid>

    <!-- 存储池模块 (Storage Pools) -->
    <div style="margin-top: 24px">
      <Storage />
    </div>

    <!-- 本地镜像模块 (Local Images) -->
    <div style="margin-top: 24px">
      <Images />
    </div>

    <!-- 容器列表模块 (Instances) -->
    <div style="margin-top: 24px">
      <Instances />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import Instances from './Instances.vue'
import Storage from './Storage.vue'
import Images from './Images.vue'
import api from '../api'

const router = useRouter()
const instancesCount = ref(0)
const runningCount = ref(0)
const stoppedCount = ref(0)
const storagePoolsCount = ref(0)

const fetchOverview = async () => {
  try {
    const resInst = await api.get('/incus/instances?recursion=1')
    const list = resInst.data.metadata || []
    instancesCount.value = list.length
    runningCount.value = list.filter((i: any) => i.status === 'Running').length
    stoppedCount.value = list.filter((i: any) => i.status !== 'Running').length

    const resPools = await api.get('/incus/storage-pools?recursion=1')
    storagePoolsCount.value = (resPools.data.metadata || []).length
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
.text-info {
  color: #70c0e8;
}
</style>
