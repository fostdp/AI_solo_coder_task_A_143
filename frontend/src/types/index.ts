export interface CrossbowConfig {
  bowArmLength: number
  bowArmStiffness: number
  stringLength: number
  stringTension: number
  stringFatigueLimit: number
  arrowMass: number
  magazineCapacity: number
  camRadius: number
  camLift: number
  frictionCoefficient: number
  gravity: number
}

export interface Crossbow {
  id: string
  name: string
  description: string
  status: 'idle' | 'running' | 'paused' | 'error'
  config: CrossbowConfig
  createdAt: string
  updatedAt: string
}

export interface SensorData {
  crossbowId: string
  timestamp: string
  stringTension: number
  bowArmDeformation: number
  magazinePosition: number
  fireRate: number
  arrowVelocity: number
  camAngle: number
  stringFatigue: number
  temperature: number
}

export interface DynamicsState {
  timestamp: string
  bowArmAngle: number
  bowArmAngularVel: number
  bowArmAngularAcc: number
  stringDisplacement: number
  stringVelocity: number
  camPosition: number
  pawlEngaged: boolean
  loadingComplete: boolean
  arrowLoaded: boolean
  forces?: Record<string, unknown>
}

export interface TrajectoryPoint {
  x: number
  y: number
  z: number
  t: number
}

export interface ArrowTrajectory {
  id: string
  crossbowId: string
  fireTime: string
  positions: TrajectoryPoint[]
  initialVelocity: number
  flightTime: number
  impactPoint: TrajectoryPoint
  createdAt: string
}

export type AlertLevel = 'info' | 'warning' | 'danger' | 'critical'

export interface Alert {
  id: string
  crossbowId: string
  type: string
  level: AlertLevel
  message: string
  value: number
  threshold: number
  createdAt: string
  acknowledged: boolean
  acknowledgedAt?: string
}

export interface AlertThresholds {
  id: string
  crossbowId: string
  stringTensionMax: number
  stringFatigueWarning: number
  fireRateMin: number
  deformationMax: number
  createdAt: string
  updatedAt: string
}

export interface RLStatus {
  isTraining: boolean
  episode: number
  totalReward: number
  averageReward: number
  epsilon: number
  currentPolicy: number[]
  trainingStartTime?: string
  bestReward: number
}

export interface RLResult {
  id: string
  crossbowId: string
  optimizedFireRate: number
  optimizedLoadingInterval: number
  fatigueReduction: number
  efficiencyImprovement: number
  sustainedFireDuration: number
  trainingEpisodes: number
  convergenceReward: number
  finalPolicy: number[]
  createdAt: string
}

export interface RLTrainingRecord {
  id: string
  crossbowId: string
  episode: number
  totalReward: number
  averageReward: number
  epsilon: number
  policy: number[]
  createdAt: string
}

export type WSMessageType =
  | 'sensor_data'
  | 'dynamics_state'
  | 'alert'
  | 'simulation_status'
  | 'rl_status'
  | 'trajectory'
  | 'connection_status'

export interface WSMessage<T = unknown> {
  type: WSMessageType
  payload: T
  timestamp: string
}

export interface StartSimulationRequest {
  simulationSpeed: number
  enableRL: boolean
  duration: number
}

export interface DataQueryRequest {
  crossbowId: string
  startTime: string
  endTime: string
  metrics: string[]
  aggregation: 'raw' | 'avg' | 'min' | 'max' | 'sum'
  interval: string
}

export interface APIResponse<T = unknown> {
  success: boolean
  message?: string
  data?: T
}

export type SimulationStatus = 'idle' | 'running' | 'paused' | 'stopped' | 'error'

export interface SimulationState {
  status: SimulationStatus
  speed: number
  currentTime: number
  enableRL: boolean
}
