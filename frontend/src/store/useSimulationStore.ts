import { create } from 'zustand'
import type {
  Crossbow,
  SensorData,
  Alert,
  RLStatus,
  SimulationStatus,
  DynamicsState,
  ArrowTrajectory
} from '../types'
import { crossbowApi, alertApi, rlApi } from '../services/api'
import wsService from '../services/websocket'

interface SimulationStore {
  crossbows: Crossbow[]
  selectedCrossbowId: string | null
  currentSensorData: SensorData | null
  sensorDataHistory: SensorData[]
  simulationStatus: SimulationStatus
  simulationSpeed: number
  enableRL: boolean
  alerts: Alert[]
  rlStatus: RLStatus | null
  dynamicsState: DynamicsState | null
  trajectories: ArrowTrajectory[]
  isLoading: boolean
  error: string | null

  fetchCrossbows: () => Promise<void>
  selectCrossbow: (id: string | null) => void
  setSimulationStatus: (status: SimulationStatus) => void
  setSimulationSpeed: (speed: number) => void
  setEnableRL: (enable: boolean) => void
  addSensorData: (data: SensorData) => void
  addAlert: (alert: Alert) => void
  acknowledgeAlert: (alertId: string) => Promise<void>
  updateRLStatus: (status: RLStatus) => void
  updateDynamicsState: (state: DynamicsState) => void
  addTrajectory: (trajectory: ArrowTrajectory) => void
  fetchInitialData: () => Promise<void>
  connectWebSocket: () => void
  disconnectWebSocket: () => void
  clearError: () => void
}

const useSimulationStore = create<SimulationStore>((set, get) => ({
  crossbows: [],
  selectedCrossbowId: null,
  currentSensorData: null,
  sensorDataHistory: [],
  simulationStatus: 'idle',
  simulationSpeed: 1,
  enableRL: false,
  alerts: [],
  rlStatus: null,
  dynamicsState: null,
  trajectories: [],
  isLoading: false,
  error: null,

  fetchCrossbows: async () => {
    set({ isLoading: true, error: null })
    try {
      const crossbows = await crossbowApi.getList()
      set({
        crossbows,
        selectedCrossbowId: get().selectedCrossbowId || (crossbows[0]?.id ?? null),
        isLoading: false
      })
    } catch (error) {
      set({ error: (error as Error).message, isLoading: false })
    }
  },

  selectCrossbow: (id: string | null) => {
    set({
      selectedCrossbowId: id,
      currentSensorData: null,
      sensorDataHistory: [],
      dynamicsState: null,
      trajectories: []
    })

    if (id) {
      alertApi.getList(id, false, 50).then(alerts => {
        set({ alerts })
      }).catch(console.error)

      rlApi.getStatus(id).then(status => {
        set({ rlStatus: status })
      }).catch(console.error)
    }
  },

  setSimulationStatus: (status: SimulationStatus) => {
    set({ simulationStatus: status })
  },

  setSimulationSpeed: (speed: number) => {
    set({ simulationSpeed: speed })
  },

  setEnableRL: (enable: boolean) => {
    set({ enableRL: enable })
  },

  addSensorData: (data: SensorData) => {
    const { sensorDataHistory } = get()
    const newHistory = [...sensorDataHistory, data].slice(-100)
    set({
      currentSensorData: data,
      sensorDataHistory: newHistory
    })
  },

  addAlert: (alert: Alert) => {
    const { alerts } = get()
    const exists = alerts.some(a => a.id === alert.id)
    if (!exists) {
      set({ alerts: [alert, ...alerts].slice(0, 100) })
    }
  },

  acknowledgeAlert: async (alertId: string) => {
    try {
      await alertApi.acknowledge(alertId)
      const { alerts } = get()
      set({
        alerts: alerts.map(a =>
          a.id === alertId ? { ...a, acknowledged: true, acknowledgedAt: new Date().toISOString() } : a
        )
      })
    } catch (error) {
      console.error('Failed to acknowledge alert:', error)
    }
  },

  updateRLStatus: (status: RLStatus) => {
    set({ rlStatus: status })
  },

  updateDynamicsState: (state: DynamicsState) => {
    set({ dynamicsState: state })
  },

  addTrajectory: (trajectory: ArrowTrajectory) => {
    const { trajectories } = get()
    set({ trajectories: [trajectory, ...trajectories].slice(0, 20) })
  },

  fetchInitialData: async () => {
    set({ isLoading: true, error: null })
    try {
      const crossbows = await crossbowApi.getList()
      const selectedId = get().selectedCrossbowId || (crossbows[0]?.id ?? null)

      let alerts: Alert[] = []
      let rlStatus: RLStatus | null = null

      if (selectedId) {
        try {
          alerts = await alertApi.getList(selectedId, false, 50)
        } catch (e) {
          console.warn('Failed to fetch alerts:', e)
        }
        try {
          rlStatus = await rlApi.getStatus(selectedId)
        } catch (e) {
          console.warn('Failed to fetch RL status:', e)
        }
      }

      set({
        crossbows,
        selectedCrossbowId: selectedId,
        alerts,
        rlStatus,
        isLoading: false
      })
    } catch (error) {
      set({ error: (error as Error).message, isLoading: false })
    }
  },

  connectWebSocket: () => {
    wsService.connect()

    wsService.on<SensorData>('sensor_data', (data) => {
      if (data.crossbowId === get().selectedCrossbowId) {
        get().addSensorData(data)
      }
    })

    wsService.on<Alert>('alert', (alert) => {
      get().addAlert(alert)
    })

    wsService.on<RLStatus>('rl_status', (status) => {
      get().updateRLStatus(status)
    })

    wsService.on<SimulationStatus>('simulation_status', (status) => {
      get().setSimulationStatus(status)
    })

    wsService.on<DynamicsState>('dynamics_state', (state) => {
      get().updateDynamicsState(state)
    })

    wsService.on<ArrowTrajectory>('trajectory', (trajectory) => {
      get().addTrajectory(trajectory)
    })
  },

  disconnectWebSocket: () => {
    wsService.disconnect()
  },

  clearError: () => {
    set({ error: null })
  }
}))

export default useSimulationStore
