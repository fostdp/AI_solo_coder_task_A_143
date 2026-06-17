import axios, { type AxiosInstance, type AxiosResponse } from 'axios'
import type {
  Crossbow,
  SensorData,
  Alert,
  RLStatus,
  RLResult,
  RLTrainingRecord,
  StartSimulationRequest,
  DataQueryRequest,
  APIResponse,
  ArrowTrajectory,
  AlertThresholds
} from '../types'

const apiClient: AxiosInstance = axios.create({
  baseURL: '/api/v1',
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
})

apiClient.interceptors.response.use(
  (response: AxiosResponse) => response,
  (error) => {
    console.error('API Error:', error)
    return Promise.reject(error)
  }
)

const handleResponse = <T>(response: AxiosResponse<APIResponse<T>>): T => {
  if (response.data.success) {
    return response.data.data as T
  }
  throw new Error(response.data.message || '请求失败')
}

export const crossbowApi = {
  getList: (): Promise<Crossbow[]> =>
    apiClient.get<APIResponse<Crossbow[]>>('/crossbows').then(handleResponse),

  getById: (id: string): Promise<Crossbow> =>
    apiClient.get<APIResponse<Crossbow>>(`/crossbows/${id}`).then(handleResponse),

  create: (data: Omit<Crossbow, 'id' | 'createdAt' | 'updatedAt'>): Promise<Crossbow> =>
    apiClient.post<APIResponse<Crossbow>>('/crossbows', data).then(handleResponse),

  update: (id: string, data: Partial<Crossbow>): Promise<Crossbow> =>
    apiClient.put<APIResponse<Crossbow>>(`/crossbows/${id}`, data).then(handleResponse),

  delete: (id: string): Promise<void> =>
    apiClient.delete<APIResponse<void>>(`/crossbows/${id}`).then(handleResponse)
}

export const simulationApi = {
  start: (crossbowId: string, request: StartSimulationRequest): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/start`, request).then(handleResponse),

  pause: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/pause`).then(handleResponse),

  resume: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/resume`).then(handleResponse),

  stop: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/stop`).then(handleResponse),

  reset: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/reset`).then(handleResponse),

  setSpeed: (crossbowId: string, speed: number): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/simulation/${crossbowId}/speed`, { speed }).then(handleResponse)
}

export const sensorApi = {
  getLatest: (crossbowId: string): Promise<SensorData> =>
    apiClient.get<APIResponse<SensorData>>(`/sensors/${crossbowId}/latest`).then(handleResponse),

  getHistory: (request: DataQueryRequest): Promise<SensorData[]> =>
    apiClient.post<APIResponse<SensorData[]>>('/sensors/history', request).then(handleResponse),

  getRealtime: (crossbowId: string, limit: number = 100): Promise<SensorData[]> =>
    apiClient.get<APIResponse<SensorData[]>>(`/sensors/${crossbowId}/realtime?limit=${limit}`).then(handleResponse)
}

export const alertApi = {
  getList: (crossbowId?: string, acknowledged?: boolean, limit: number = 100): Promise<Alert[]> => {
    const params = new URLSearchParams()
    if (crossbowId) params.append('crossbowId', crossbowId)
    if (acknowledged !== undefined) params.append('acknowledged', String(acknowledged))
    params.append('limit', String(limit))
    return apiClient.get<APIResponse<Alert[]>>(`/alerts?${params.toString()}`).then(handleResponse)
  },

  acknowledge: (id: string): Promise<Alert> =>
    apiClient.post<APIResponse<Alert>>(`/alerts/${id}/acknowledge`).then(handleResponse),

  getThresholds: (crossbowId: string): Promise<AlertThresholds> =>
    apiClient.get<APIResponse<AlertThresholds>>(`/alerts/thresholds/${crossbowId}`).then(handleResponse),

  updateThresholds: (crossbowId: string, thresholds: Partial<AlertThresholds>): Promise<AlertThresholds> =>
    apiClient.put<APIResponse<AlertThresholds>>(`/alerts/thresholds/${crossbowId}`, thresholds).then(handleResponse)
}

export const rlApi = {
  getStatus: (crossbowId: string): Promise<RLStatus> =>
    apiClient.get<APIResponse<RLStatus>>(`/rl/${crossbowId}/status`).then(handleResponse),

  startTraining: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/rl/${crossbowId}/start`).then(handleResponse),

  stopTraining: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/rl/${crossbowId}/stop`).then(handleResponse),

  getResult: (crossbowId: string): Promise<RLResult> =>
    apiClient.get<APIResponse<RLResult>>(`/rl/${crossbowId}/result`).then(handleResponse),

  getTrainingHistory: (crossbowId: string, limit: number = 100): Promise<RLTrainingRecord[]> =>
    apiClient.get<APIResponse<RLTrainingRecord[]>>(`/rl/${crossbowId}/history?limit=${limit}`).then(handleResponse),

  applyPolicy: (crossbowId: string): Promise<void> =>
    apiClient.post<APIResponse<void>>(`/rl/${crossbowId}/apply`).then(handleResponse)
}

export const trajectoryApi = {
  getLatest: (crossbowId: string, limit: number = 10): Promise<ArrowTrajectory[]> =>
    apiClient.get<APIResponse<ArrowTrajectory[]>>(`/trajectories/${crossbowId}/latest?limit=${limit}`).then(handleResponse),

  getById: (id: string): Promise<ArrowTrajectory> =>
    apiClient.get<APIResponse<ArrowTrajectory>>(`/trajectories/${id}`).then(handleResponse)
}

export default apiClient
