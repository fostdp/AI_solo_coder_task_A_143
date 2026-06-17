import React from 'react'
import { List, Button, Tag, Typography, Empty } from 'antd'
import {
  InfoCircleOutlined,
  WarningOutlined,
  ExclamationCircleOutlined,
  CloseCircleOutlined,
  CheckOutlined
} from '@ant-design/icons'
import dayjs from 'dayjs'
import type { Alert, AlertLevel } from '../types'

const { Text, Paragraph } = Typography

interface AlertPanelProps {
  alerts: Alert[]
  onAcknowledge: (alertId: string) => void
  maxItems?: number
  showTitle?: boolean
}

const levelConfig: Record<AlertLevel, {
  color: string
  bgColor: string
  borderColor: string
  icon: React.ReactNode
  label: string
}> = {
  info: {
    color: '#1890ff',
    bgColor: 'rgba(24, 144, 255, 0.1)',
    borderColor: '#1890ff',
    icon: <InfoCircleOutlined />,
    label: '信息'
  },
  warning: {
    color: '#CD853F',
    bgColor: 'rgba(205, 133, 63, 0.15)',
    borderColor: '#CD853F',
    icon: <WarningOutlined />,
    label: '警告'
  },
  danger: {
    color: '#D2691E',
    bgColor: 'rgba(210, 105, 30, 0.15)',
    borderColor: '#D2691E',
    icon: <ExclamationCircleOutlined />,
    label: '危险'
  },
  critical: {
    color: '#8B0000',
    bgColor: 'rgba(139, 0, 0, 0.2)',
    borderColor: '#8B0000',
    icon: <CloseCircleOutlined />,
    label: '严重'
  }
}

const AlertPanel: React.FC<AlertPanelProps> = ({
  alerts,
  onAcknowledge,
  maxItems = 10,
  showTitle = true
}) => {
  const displayAlerts = alerts.slice(0, maxItems)
  const unreadCount = alerts.filter(a => !a.acknowledged).length

  const getAlertIcon = (level: AlertLevel) => {
    const config = levelConfig[level]
    return React.cloneElement(config.icon as React.ReactElement, {
      style: { color: config.color, fontSize: 18 }
    })
  }

  return (
    <div
      style={{
        background: 'linear-gradient(180deg, #2a1f18 0%, #1a1410 100%)',
        border: '1px solid #8B4513',
        borderRadius: 6,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden'
      }}
    >
      {showTitle && (
        <div
          style={{
            padding: '12px 16px',
            borderBottom: '1px solid #8B4513',
            display: 'flex',
            justifyContent: 'space-between',
            alignItems: 'center',
            background: 'rgba(139, 69, 19, 0.2)'
          }}
        >
          <Text strong style={{ color: '#B8860B', fontSize: 14, fontFamily: "'Noto Serif SC', serif" }}>
            实时告警
          </Text>
          {unreadCount > 0 && (
            <Tag color="#8B0000" style={{ margin: 0 }}>
              {unreadCount} 条未确认
            </Tag>
          )}
        </div>
      )}

      <div style={{ flex: 1, overflow: 'auto', padding: 8 }}>
        {displayAlerts.length === 0 ? (
          <Empty
            description={
              <Text style={{ color: '#7a6a5a' }}>暂无告警</Text>
            }
            image={Empty.PRESENTED_IMAGE_SIMPLE}
            style={{ padding: '40px 0' }}
          />
        ) : (
          <List
            dataSource={displayAlerts}
            renderItem={(alert) => {
              const config = levelConfig[alert.level]
              return (
                <List.Item
                  key={alert.id}
                  style={{
                    padding: '12px',
                    marginBottom: 8,
                    background: alert.acknowledged ? 'rgba(42, 31, 24, 0.5)' : config.bgColor,
                    border: `1px solid ${alert.acknowledged ? '#5a4a3a' : config.borderColor}`,
                    borderRadius: 4,
                    opacity: alert.acknowledged ? 0.6 : 1,
                    transition: 'all 0.3s ease'
                  }}
                >
                  <div style={{ width: '100%' }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 8 }}>
                      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                        {getAlertIcon(alert.level)}
                        <Tag
                          color={config.color}
                          style={{
                            margin: 0,
                            borderColor: config.color,
                            fontWeight: 500
                          }}
                        >
                          {config.label}
                        </Tag>
                        <Text style={{ color: '#a89888', fontSize: 12 }}>
                          {dayjs(alert.createdAt).format('MM-DD HH:mm:ss')}
                        </Text>
                      </div>
                      {!alert.acknowledged && (
                        <Button
                          type="primary"
                          size="small"
                          icon={<CheckOutlined />}
                          onClick={() => onAcknowledge(alert.id)}
                          style={{
                            background: 'linear-gradient(135deg, #2E8B57 0%, #1a5a37 100%)',
                            border: '1px solid #2E8B57'
                          }}
                        >
                          确认
                        </Button>
                      )}
                    </div>

                    <Paragraph
                      style={{
                        margin: '0 0 8px 0',
                        color: '#d4c4a8',
                        fontSize: 13,
                        fontFamily: "'Noto Serif SC', serif"
                      }}
                    >
                      {alert.message}
                    </Paragraph>

                    <div style={{ display: 'flex', gap: 16, fontSize: 12 }}>
                      <Text style={{ color: '#7a6a5a' }}>
                        类型: <span style={{ color: '#a89888' }}>{alert.type}</span>
                      </Text>
                      <Text style={{ color: '#7a6a5a' }}>
                        当前值: <span style={{ color: config.color, fontWeight: 500 }}>{alert.value.toFixed(2)}</span>
                      </Text>
                      <Text style={{ color: '#7a6a5a' }}>
                        阈值: <span style={{ color: '#a89888' }}>{alert.threshold.toFixed(2)}</span>
                      </Text>
                    </div>
                  </div>
                </List.Item>
              )
            }}
          />
        )}
      </div>
    </div>
  )
}

export default AlertPanel
