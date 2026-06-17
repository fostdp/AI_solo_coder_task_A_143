import type {
  WSMessage,
  WSMessageType
} from '../types'

type MessageHandler<T = unknown> = (message: T) => void
type ConnectionHandler = (status: ConnectionStatus) => void

export type ConnectionStatus = 'connected' | 'disconnected' | 'connecting' | 'reconnecting'

interface WebSocketServiceOptions {
  url: string
  reconnectInterval?: number
  maxReconnectAttempts?: number
  heartbeatInterval?: number
}

class WebSocketService {
  private ws: WebSocket | null = null
  private url: string
  private reconnectInterval: number
  private maxReconnectAttempts: number
  private heartbeatInterval: number
  private reconnectAttempts: number = 0
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  private connectionStatus: ConnectionStatus = 'disconnected'
  private messageHandlers: Map<WSMessageType, Set<MessageHandler>> = new Map()
  private connectionHandlers: Set<ConnectionHandler> = new Set()
  private manualClose: boolean = false

  constructor(options: WebSocketServiceOptions) {
    this.url = options.url
    this.reconnectInterval = options.reconnectInterval || 3000
    this.maxReconnectAttempts = options.maxReconnectAttempts || 10
    this.heartbeatInterval = options.heartbeatInterval || 30000
  }

  connect(): void {
    if (this.ws?.readyState === WebSocket.OPEN || this.ws?.readyState === WebSocket.CONNECTING) {
      return
    }

    this.setConnectionStatus('connecting')
    this.manualClose = false

    try {
      const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
      const wsUrl = this.url.startsWith('ws') ? this.url : `${protocol}//${window.location.host}${this.url}`
      this.ws = new WebSocket(wsUrl)

      this.ws.onopen = this.handleOpen.bind(this)
      this.ws.onmessage = this.handleMessage.bind(this)
      this.ws.onerror = this.handleError.bind(this)
      this.ws.onclose = this.handleClose.bind(this)
    } catch (error) {
      console.error('WebSocket connection error:', error)
      this.scheduleReconnect()
    }
  }

  disconnect(): void {
    this.manualClose = true
    this.stopHeartbeat()
    if (this.ws) {
      this.ws.close(1000, 'Manual close')
      this.ws = null
    }
    this.setConnectionStatus('disconnected')
    this.reconnectAttempts = 0
  }

  reconnect(): void {
    this.disconnect()
    this.connect()
  }

  send<T = unknown>(type: WSMessageType, payload: T): void {
    if (this.ws?.readyState !== WebSocket.OPEN) {
      console.warn('WebSocket is not connected. Message queued.')
      return
    }

    const message: WSMessage<T> = {
      type,
      payload,
      timestamp: new Date().toISOString()
    }

    this.ws.send(JSON.stringify(message))
  }

  on<T>(type: WSMessageType, handler: MessageHandler<T>): () => void {
    if (!this.messageHandlers.has(type)) {
      this.messageHandlers.set(type, new Set())
    }
    this.messageHandlers.get(type)!.add(handler as MessageHandler)

    return () => {
      this.messageHandlers.get(type)?.delete(handler as MessageHandler)
    }
  }

  onConnection(handler: ConnectionHandler): () => void {
    this.connectionHandlers.add(handler)
    handler(this.connectionStatus)
    return () => {
      this.connectionHandlers.delete(handler)
    }
  }

  off<T>(type: WSMessageType, handler: MessageHandler<T>): void {
    this.messageHandlers.get(type)?.delete(handler as MessageHandler)
  }

  getStatus(): ConnectionStatus {
    return this.connectionStatus
  }

  private handleOpen(): void {
    console.log('WebSocket connected')
    this.reconnectAttempts = 0
    this.setConnectionStatus('connected')
    this.startHeartbeat()
  }

  private handleMessage(event: MessageEvent): void {
    try {
      const message: WSMessage = JSON.parse(event.data)

      const handlers = this.messageHandlers.get(message.type)
      if (handlers) {
        handlers.forEach(handler => handler(message.payload))
      }

      const allHandlers = this.messageHandlers.get('*' as WSMessageType)
      if (allHandlers) {
        allHandlers.forEach(handler => handler(message))
      }
    } catch (error) {
      console.error('Error parsing WebSocket message:', error)
    }
  }

  private handleError(event: Event): void {
    console.error('WebSocket error:', event)
  }

  private handleClose(event: CloseEvent): void {
    console.log('WebSocket closed:', event.code, event.reason)
    this.stopHeartbeat()
    this.setConnectionStatus('disconnected')

    if (!this.manualClose) {
      this.scheduleReconnect()
    }
  }

  private scheduleReconnect(): void {
    if (this.reconnectAttempts >= this.maxReconnectAttempts) {
      console.error('Max reconnect attempts reached')
      this.setConnectionStatus('disconnected')
      return
    }

    this.reconnectAttempts++
    this.setConnectionStatus('reconnecting')

    const delay = this.reconnectInterval * Math.min(this.reconnectAttempts, 5)

    console.log(`Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts}/${this.maxReconnectAttempts})`)

    setTimeout(() => {
      if (!this.manualClose) {
        this.connect()
      }
    }, delay)
  }

  private startHeartbeat(): void {
    this.stopHeartbeat()
    this.heartbeatTimer = setInterval(() => {
      if (this.ws?.readyState === WebSocket.OPEN) {
        this.send('connection_status', { type: 'heartbeat' })
      }
    }, this.heartbeatInterval)
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  private setConnectionStatus(status: ConnectionStatus): void {
    this.connectionStatus = status
    this.connectionHandlers.forEach(handler => handler(status))
  }
}

const wsService = new WebSocketService({
  url: '/ws'
})

export { wsService }
export default wsService
