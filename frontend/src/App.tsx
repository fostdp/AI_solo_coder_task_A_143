import React, { useState, useEffect } from 'react'
import { Layout, Menu, Badge, Typography, Space, Tag, Dropdown, Avatar } from 'antd'
import {
  DashboardOutlined,
  BarChartOutlined,
  BellOutlined,
  SettingOutlined,
  CaretDownOutlined,
  UserOutlined,
  WifiOutlined,
  ClockCircleOutlined
} from '@ant-design/icons'
import type { MenuProps } from 'antd'
import dayjs from 'dayjs'
import SimulationPage from './pages/SimulationPage'

const { Header, Sider, Content, Footer } = Layout
const { Title, Text } = Typography

type MenuKey = 'simulation' | 'analysis' | 'alerts' | 'settings'

function App() {
  const [selectedKey, setSelectedKey] = useState<MenuKey>('simulation')
  const [connectionStatus, setConnectionStatus] = useState<'connected' | 'disconnected' | 'connecting'>('connected')
  const [simulationSpeed, setSimulationSpeed] = useState(1)
  const [currentTime, setCurrentTime] = useState(dayjs())
  const [unreadAlerts, setUnreadAlerts] = useState(3)

  useEffect(() => {
    const timer = setInterval(() => {
      setCurrentTime(dayjs())
    }, 1000)
    return () => clearInterval(timer)
  }, [])

  const menuItems: MenuProps['items'] = [
    {
      key: 'simulation',
      icon: <DashboardOutlined />,
      label: '仿真监控'
    },
    {
      key: 'analysis',
      icon: <BarChartOutlined />,
      label: '数据分析'
    },
    {
      key: 'alerts',
      icon: <Badge count={unreadAlerts} size="small" offset={[8, -2]}><BellOutlined /></Badge>,
      label: '告警管理'
    },
    {
      key: 'settings',
      icon: <SettingOutlined />,
      label: '系统设置'
    }
  ]

  const userMenuItems: MenuProps['items'] = [
    { key: 'profile', label: '个人信息' },
    { key: 'settings', label: '账户设置' },
    { type: 'divider' },
    { key: 'logout', label: '退出登录', danger: true }
  ]

  const statusColor = connectionStatus === 'connected' ? 'success' : connectionStatus === 'connecting' ? 'warning' : 'error'
  const statusText = connectionStatus === 'connected' ? '已连接' : connectionStatus === 'connecting' ? '连接中' : '已断开'

  const renderContent = () => {
    switch (selectedKey) {
      case 'simulation':
        return <SimulationPage onSpeedChange={setSimulationSpeed} />
      case 'analysis':
        return <div style={{ padding: 24, color: '#d4c4a8' }}>数据分析页面（开发中）</div>
      case 'alerts':
        return <div style={{ padding: 24, color: '#d4c4a8' }}>告警管理页面（开发中）</div>
      case 'settings':
        return <div style={{ padding: 24, color: '#d4c4a8' }}>系统设置页面（开发中）</div>
      default:
        return <SimulationPage onSpeedChange={setSimulationSpeed} />
    }
  }

  return (
    <Layout style={{ height: '100vh', background: '#1a1410' }}>
      <Header
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '0 24px',
          background: 'linear-gradient(180deg, #2a1f18 0%, #1a1410 100%)',
          borderBottom: '1px solid #8B4513',
          height: 64
        }}
      >
        <Space size={16}>
          <div
            style={{
              width: 40,
              height: 40,
              borderRadius: '50%',
              background: 'linear-gradient(135deg, #B8860B 0%, #8B4513 100%)',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              boxShadow: '0 0 20px rgba(184, 134, 11, 0.3)'
            }}
          >
            <span style={{ fontFamily: "'Ma Shan Zheng', cursive", fontSize: 20, color: '#fff' }}>弩</span>
          </div>
          <Title
            level={4}
            style={{
              margin: 0,
              fontFamily: "'Ma Shan Zheng', cursive",
              color: '#B8860B',
              letterSpacing: 4
            }}
          >
            连弩仿真监控系统
          </Title>
        </Space>

        <Space size={24}>
          <Space size={8}>
            <Tag color={statusColor} icon={<WifiOutlined />}>
              {statusText}
            </Tag>
            <Badge count={unreadAlerts} offset={[2, -2]}>
              <BellOutlined style={{ fontSize: 18, color: '#d4c4a8', cursor: 'pointer' }} />
            </Badge>
          </Space>

          <Dropdown menu={{ items: userMenuItems }} placement="bottomRight">
            <Space style={{ cursor: 'pointer', padding: '4px 8px', borderRadius: 4 }}>
              <Avatar size="small" icon={<UserOutlined />} style={{ background: '#8B4513' }} />
              <Text style={{ color: '#d4c4a8' }}>管理员</Text>
              <CaretDownOutlined style={{ color: '#a89888', fontSize: 12 }} />
            </Space>
          </Dropdown>
        </Space>
      </Header>

      <Layout>
        <Sider
          width={220}
          style={{
            background: '#2a1f18',
            borderRight: '1px solid #8B4513'
          }}
        >
          <Menu
            mode="inline"
            selectedKeys={[selectedKey]}
            items={menuItems}
            style={{
              height: '100%',
              borderRight: 'none',
              background: 'transparent',
              paddingTop: 16
            }}
            theme="dark"
            onClick={({ key }) => setSelectedKey(key as MenuKey)}
          />
        </Sider>

        <Content style={{ background: '#1a1410', overflow: 'hidden', display: 'flex', flexDirection: 'column' }}>
          {renderContent()}
        </Content>
      </Layout>

      <Footer
        style={{
          padding: '8px 24px',
          background: '#2a1f18',
          borderTop: '1px solid #8B4513',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          height: 40
        }}
      >
        <Space size={24}>
          <Space size={8}>
            <WifiOutlined style={{ color: statusColor === 'success' ? '#2E8B57' : '#8B0000' }} />
            <Text style={{ color: '#a89888', fontSize: 12 }}>连接状态: {statusText}</Text>
          </Space>
          <Space size={8}>
            <span style={{ color: '#B8860B', fontSize: 12 }}>×</span>
            <Text style={{ color: '#a89888', fontSize: 12 }}>仿真速度: {simulationSpeed}x</Text>
          </Space>
        </Space>
        <Space size={8}>
          <ClockCircleOutlined style={{ color: '#B8860B' }} />
          <Text style={{ color: '#a89888', fontSize: 12 }}>{currentTime.format('YYYY-MM-DD HH:mm:ss')}</Text>
        </Space>
      </Footer>
    </Layout>
  )
}

export default App
