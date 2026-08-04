<template>
  <div class="login-container">
    <n-card class="login-card" title="🚀 Incus Pilot">
      <n-form :model="formValue">
        <n-form-item label="Username">
          <n-input v-model:value="formValue.username" placeholder="admin" />
        </n-form-item>
        <n-form-item label="Password">
          <n-input
            v-model:value="formValue.password"
            type="password"
            placeholder="admin123456"
            @keyup.enter="handleLogin"
          />
        </n-form-item>
        <n-button type="primary" block :loading="loading" @click="handleLogin">
          Login
        </n-button>
      </n-form>
    </n-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useMessage } from 'naive-ui'
import api from '../api'

const router = useRouter()
const message = useMessage()
const loading = ref(false)

const formValue = ref({
  username: 'admin',
  password: ''
})

const handleLogin = async () => {
  if (!formValue.value.password) {
    message.warning('Please enter password')
    return
  }
  loading.value = true
  try {
    const res = await api.post('/login', formValue.value)
    localStorage.setItem('token', res.data.token)
    message.success('Welcome to Incus Pilot')
    router.push('/')
  } catch (err: any) {
    message.error(err.response?.data?.error || 'Login failed')
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: radial-gradient(circle at center, #1e1e24 0%, #101014 100%);
}
.login-card {
  width: 360px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
}
</style>
