import React, { useEffect, useRef, useState, useCallback } from 'react'
import { Canvas, useFrame } from '@react-three/fiber'
import { OrbitControls, Environment, Grid, ContactShadows } from '@react-three/drei'
import * as THREE from 'three'
import { Button, Slider, Switch, Space, Typography, Select, Card, Row, Col, Tag } from 'antd'
import {
  PlayCircleOutlined,
  PauseCircleOutlined,
  ReloadOutlined,
  RobotOutlined,
  SettingOutlined
} from '@ant-design/icons'
import Gauge from '../components/Gauge'
import AlertPanel from '../components/AlertPanel'
import RealtimeChart from '../components/RealtimeChart'
import useSimulationStore from '../store/useSimulationStore'
import { simulationApi, rlApi } from '../services/api'
import type { SimulationStatus } from '../types'

const { Text } = Typography
const { Option } = Select

interface SimulationPageProps {
  onSpeedChange?: (speed: number) => void
}

const Crossbow3D: React.FC = () => {
  const groupRef = useRef<THREE.Group>(null)
  const bowArmsRef = useRef<THREE.Group>(null)
  const stringRef = useRef<THREE.Line>(null)
  const camRef = useRef<THREE.Mesh>(null)
  const arrowRef = useRef<THREE.Mesh>(null)

  useFrame((state, delta) => {
    if (groupRef.current) {
      groupRef.current.rotation.y = Math.sin(state.clock.elapsedTime * 0.2) * 0.1
    }
    if (bowArmsRef.current) {
      const angle = Math.sin(state.clock.elapsedTime * 2) * 0.2
      bowArmsRef.current.rotation.z = angle
    }
    if (camRef.current) {
      camRef.current.rotation.z = state.clock.elapsedTime * 3
    }
  })

  return (
    <group ref={groupRef} position={[0, 0.5, 0]}>
      <mesh position={[0, 0, 0]} castShadow receiveShadow>
        <boxGeometry args={[0.1, 0.08, 1.2]} />
        <meshStandardMaterial
          color="#8B4513"
          metalness={0.3}
          roughness={0.7}
        />
      </mesh>

      <group ref={bowArmsRef}>
        <mesh position={[-0.5, 0.2, -0.4]} rotation={[0, 0, 0.5]} castShadow>
          <boxGeometry args={[0.05, 0.5, 0.03]} />
          <meshStandardMaterial
            color="#654321"
            metalness={0.2}
            roughness={0.8}
          />
        </mesh>
        <mesh position={[0.5, 0.2, -0.4]} rotation={[0, 0, -0.5]} castShadow>
          <boxGeometry args={[0.05, 0.5, 0.03]} />
          <meshStandardMaterial
            color="#654321"
            metalness={0.2}
            roughness={0.8}
          />
        </mesh>
      </group>

      <mesh ref={camRef} position={[0, 0, -0.3]} castShadow>
        <cylinderGeometry args={[0.08, 0.08, 0.1, 32]} />
        <meshStandardMaterial
          color="#B8860B"
          metalness={0.8}
          roughness={0.2}
        />
      </mesh>

      <mesh ref={arrowRef} position={[0, 0, 0.4]} castShadow>
        <coneGeometry args={[0.02, 0.3, 8]} />
        <meshStandardMaterial
          color="#CD853F"
          metalness={0.4}
          roughness={0.6}
        />
      </mesh>

      <mesh position={[0, 0, 0.7]} rotation={[Math.PI / 2, 0, 0]} castShadow>
        <torusGeometry args={[0.04, 0.01, 8, 16]} />
        <meshStandardMaterial
          color="#8B4513"
          metalness={0.6}
          roughness={0.4}
        />
      </mesh>
    </group>
  )
}

const Scene: React.FC = () => {
  return (
    <>
      <ambientLight intensity={0.4} />
      <directionalLight
        position={[5, 10, 5]}
        intensity={1}
        castShadow
        shadow-mapSize-width={1024}
        shadow-mapSize-height={1024}
      />
      <pointLight position={[-5, 5, -5]} intensity={0.5} color="#B8860B" />
      <pointLight position={[5, 3, 5]} intensity={0.3} color="#8B4513" />

      <Crossbow3D />

      <Grid
        position={[0, -0.5, 0]}
        args={[20, 20]}
        cellSize={0.5}
        cellThickness={0.5}
        cellColor="#5a4a3a"
        sectionSize={2}
        sectionThickness={1}
        sectionColor="#8B4513"
        fadeDistance={25}
        fadeStrength={1}
        followCamera={false}
        infiniteGrid
      />

      <ContactShadows
        position={[0, -0.5, 0]}
        opacity={0.5}
        scale={10}
        blur={2}
        far={4}
      />

      <OrbitControls
        enableDamping
        dampingFactor={0.05}
        minDistance={2}
        maxDistance={10}
        target={[0, 0.5, 0]}
      />

      <Environment preset="sunset" />
    </>
  )
}

const SimulationPage: React.FC<SimulationPageProps> = ({ onSpeedChange }) => {
  const {
    crossbows,
    selectedCrossbowId,
    currentSensorData,
    sensorDataHistory,
    simulationStatus,
    simulationSpeed,
    enableRL,
    alerts,
    rlStatus,
    selectCrossbow,
    setSimulationStatus,
    setSimulationSpeed,
    setEnableRL,
    acknowledgeAlert,
    fetchInitialData,
    connectWebSocket,
    disconnectWebSocket
  } = useSimulationStore()

  const [isPlaying, setIsPlaying] = useState(false)

  useEffect(() => {
    fetchInitialData()
    connectWebSocket()
    return () => {
      disconnectWebSocket()
    }
  }, [])

  const handlePlayPause = useCallback(async () => {
    if (!selectedCrossbowId) return

    try {
      if (simulationStatus === 'running') {
        await simulationApi.pause(selectedCrossbowId)
        setSimulationStatus('paused')
        setIsPlaying(false)
      } else if (simulationStatus === 'paused') {
        await simulationApi.resume(selectedCrossbowId)
        setSimulationStatus('running')
        setIsPlaying(true)
      } else {
        await simulationApi.start(selectedCrossbowId, {
          simulationSpeed,
          enableRL,
          duration: 3600
        })
        setSimulationStatus('running')
        setIsPlaying(true)
      }
    } catch (error) {
      console.error('Failed to control simulation:', error)
    }
  }, [selectedCrossbowId, simulationStatus, simulationSpeed, enableRL])

  const handleReset = useCallback(async () => {
    if (!selectedCrossbowId) return
    try {
      await simulationApi.reset(selectedCrossbowId)
      setSimulationStatus('idle')
      setIsPlaying(false)
    } catch (error) {
      console.error('Failed to reset simulation:', error)
    }
  }, [selectedCrossbowId])

  const handleSpeedChange = useCallback(async (value: number) => {
    setSimulationSpeed(value)
    onSpeedChange?.(value)
    if (selectedCrossbowId && simulationStatus === 'running') {
      try {
        await simulationApi.setSpeed(selectedCrossbowId, value)
      } catch (error) {
        console.error('Failed to set speed:', error)
      }
    }
  }, [selectedCrossbowId, simulationStatus, onSpeedChange])

  const handleRLToggle = useCallback(async (checked: boolean) => {
    setEnableRL(checked)
    if (!selectedCrossbowId) return
    try {
      if (checked) {
        await rlApi.startTraining(selectedCrossbowId)
      } else {
        await rlApi.stopTraining(selectedCrossbowId)
      }
    } catch (error) {
      console.error('Failed to toggle RL:', error)
    }
  }, [selectedCrossbowId])

  const getStatusColor = (status: SimulationStatus) => {
    switch (status) {
      case 'running': return 'success'
      case 'paused': return 'warning'
      case 'error': return 'error'
      default: return 'default'
    }
  }

  const getStatusText = (status: SimulationStatus) => {
    switch (status) {
      case 'running': return '运行中'
      case 'paused': return '已暂停'
      case 'stopped': return '已停止'
      case 'error': return '错误'
      default: return '待命'
    }
  }

  const chartData = sensorDataHistory.map(d => ({
    time: d.timestamp,
    stringTension: d.stringTension,
    bowArmDeformation: d.bowArmDeformation,
    fireRate: d.fireRate,
    stringFatigue: d.stringFatigue
  }))

  const selectedCrossbow = crossbows.find(c => c.id === selectedCrossbowId)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%', overflow: 'hidden' }}>
      <div style={{ padding: '12px 24px', borderBottom: '1px solid #8B4513', display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Space size={16}>
          <Select
            value={selectedCrossbowId}
            onChange={selectCrossbow}
            placeholder="选择连弩"
            bordered={false}
            style={{ width: 240, background: '#2a1f18', borderRadius: 4 }}
          >
            {crossbows.map(crossbow => (
              <Option key={crossbow.id} value={crossbow.id}>
                {crossbow.name}
              </Option>
            ))}
          </Select>
          {selectedCrossbow && (
            <Tag color={getStatusColor(selectedCrossbow.status as SimulationStatus)}>
              {getStatusText(selectedCrossbow.status as SimulationStatus)}
            </Tag>
          )}
        </Space>

        <Space size={16}>
          {rlStatus?.isTraining && (
            <Tag color="#B8860B" icon={<RobotOutlined />}>
              RL训练中 - 第 {rlStatus.episode} 轮, 奖励: {rlStatus.averageReward.toFixed(2)}
            </Tag>
          )}
        </Space>
      </div>

      <div style={{ flex: 1, display: 'flex', overflow: 'hidden' }}>
        <div style={{ width: '70%', position: 'relative', background: 'radial-gradient(ellipse at center, #2a1f18 0%, #1a1410 70%)' }}>
          <Canvas
            shadows
            camera={{ position: [3, 2, 3], fov: 50 }}
            gl={{ antialias: true }}
          >
            <color attach="background" args={['#1a1410']} />
            <fog attach="fog" args={['#1a1410', 5, 20]} />
            <Scene />
          </Canvas>

          <div style={{ position: 'absolute', top: 16, left: 16, right: 16, display: 'flex', justifyContent: 'space-between' }}>
            <Card
              size="small"
              style={{ background: 'rgba(26, 20, 16, 0.85)', border: '1px solid #8B4513', backdropFilter: 'blur(10px)' }}
            >
              <Text style={{ color: '#a89888', fontSize: 12 }}>当前箭矢速度</Text>
              <div style={{ fontSize: 24, fontWeight: 'bold', color: '#B8860B', fontFamily: "'Noto Serif SC', serif" }}>
                {currentSensorData?.arrowVelocity.toFixed(1) || '0.0'}
                <span style={{ fontSize: 14, color: '#a89888', marginLeft: 4 }}>m/s</span>
              </div>
            </Card>

            <Card
              size="small"
              style={{ background: 'rgba(26, 20, 16, 0.85)', border: '1px solid #8B4513', backdropFilter: 'blur(10px)' }}
            >
              <Text style={{ color: '#a89888', fontSize: 12 }}>弓弦疲劳度</Text>
              <div style={{ fontSize: 24, fontWeight: 'bold', color: currentSensorData && currentSensorData.stringFatigue > 70 ? '#8B0000' : '#2E8B57', fontFamily: "'Noto Serif SC', serif" }}>
                {currentSensorData?.stringFatigue.toFixed(1) || '0.0'}
                <span style={{ fontSize: 14, color: '#a89888', marginLeft: 4 }}>%</span>
              </div>
            </Card>

            <Card
              size="small"
              style={{ background: 'rgba(26, 20, 16, 0.85)', border: '1px solid #8B4513', backdropFilter: 'blur(10px)' }}
            >
              <Text style={{ color: '#a89888', fontSize: 12 }}>温度</Text>
              <div style={{ fontSize: 24, fontWeight: 'bold', color: '#CD853F', fontFamily: "'Noto Serif SC', serif" }}>
                {currentSensorData?.temperature.toFixed(1) || '0.0'}
                <span style={{ fontSize: 14, color: '#a89888', marginLeft: 4 }}>°C</span>
              </div>
            </Card>
          </div>
        </div>

        <div style={{ width: '30%', display: 'flex', flexDirection: 'column', overflow: 'hidden', borderLeft: '1px solid #8B4513' }}>
          <div style={{ flex: 1, padding: 16, overflow: 'auto' }}>
            <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
              <Col span={12}>
                <Gauge
                  value={currentSensorData?.stringTension || 0}
                  min={0}
                  max={500}
                  title="弓弦张力"
                  unit="N"
                  warningThreshold={350}
                  dangerThreshold={450}
                  size={140}
                />
              </Col>
              <Col span={12}>
                <Gauge
                  value={currentSensorData?.bowArmDeformation || 0}
                  min={0}
                  max={100}
                  title="弩臂变形"
                  unit="mm"
                  warningThreshold={60}
                  dangerThreshold={85}
                  size={140}
                />
              </Col>
              <Col span={12}>
                <Gauge
                  value={currentSensorData?.magazinePosition || 0}
                  min={0}
                  max={10}
                  title="箭匣位置"
                  unit="发"
                  size={140}
                />
              </Col>
              <Col span={12}>
                <Gauge
                  value={currentSensorData?.fireRate || 0}
                  min={0}
                  max={10}
                  title="射速"
                  unit="发/秒"
                  warningThreshold={2}
                  dangerThreshold={1}
                  size={140}
                />
              </Col>
            </Row>

            <div style={{ marginBottom: 16 }}>
              <RealtimeChart
                data={chartData}
                series={[
                  { key: 'stringTension', name: '张力', color: '#B8860B', unit: 'N' },
                  { key: 'bowArmDeformation', name: '变形', color: '#CD853F', unit: 'mm' }
                ]}
                title="张力与变形趋势"
                height={160}
                thresholdLines={[
                  { value: 350, name: '张力警告', color: '#CD853F' },
                  { value: 450, name: '张力危险', color: '#8B0000' }
                ]}
              />
            </div>

            <div style={{ marginBottom: 16 }}>
              <RealtimeChart
                data={chartData}
                series={[
                  { key: 'fireRate', name: '射速', color: '#2E8B57', unit: '发/秒' },
                  { key: 'stringFatigue', name: '疲劳度', color: '#8B4513', unit: '%' }
                ]}
                title="射速与疲劳度趋势"
                height={160}
              />
            </div>

            <div style={{ height: 280 }}>
              <AlertPanel
                alerts={alerts}
                onAcknowledge={acknowledgeAlert}
                maxItems={5}
              />
            </div>
          </div>
        </div>
      </div>

      <div
        style={{
          padding: '12px 24px',
          background: 'linear-gradient(180deg, #1a1410 0%, #2a1f18 100%)',
          borderTop: '1px solid #8B4513',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center'
        }}
      >
        <Space size={16}>
          <Button
            type="primary"
            size="large"
            icon={isPlaying ? <PauseCircleOutlined /> : <PlayCircleOutlined />}
            onClick={handlePlayPause}
            disabled={!selectedCrossbowId}
            style={{
              background: isPlaying
                ? 'linear-gradient(135deg, #CD853F 0%, #8B4513 100%)'
                : 'linear-gradient(135deg, #2E8B57 0%, #1a5a37 100%)',
              border: 'none',
              minWidth: 120,
              fontSize: 14,
              fontWeight: 500,
              boxShadow: '0 4px 15px rgba(0, 0, 0, 0.3)'
            }}
          >
            {isPlaying ? '暂停' : simulationStatus === 'paused' ? '继续' : '开始'}
          </Button>

          <Button
            size="large"
            icon={<ReloadOutlined />}
            onClick={handleReset}
            disabled={!selectedCrossbowId}
            style={{
              background: 'linear-gradient(135deg, #3a2f28 0%, #2a1f18 100%)',
              border: '1px solid #8B4513',
              color: '#d4c4a8'
            }}
          >
            重置
          </Button>

          <Space size={8} style={{ marginLeft: 16 }}>
            <Text style={{ color: '#a89888', fontSize: 13 }}>速度:</Text>
            <Slider
              min={0.5}
              max={3}
              step={0.5}
              value={simulationSpeed}
              onChange={handleSpeedChange}
              style={{ width: 150 }}
              tooltip={{
                formatter: (value) => `${value}x`
              }}
            />
            <Text strong style={{ color: '#B8860B', fontSize: 14, minWidth: 40 }}>{simulationSpeed}x</Text>
          </Space>
        </Space>

        <Space size={24}>
          <Space size={8}>
            <Switch
              checked={enableRL}
              onChange={handleRLToggle}
              disabled={!selectedCrossbowId}
              checkedChildren={<RobotOutlined />}
              unCheckedChildren={<RobotOutlined />}
              style={{
                background: enableRL ? '#B8860B' : undefined
              }}
            />
            <Text style={{ color: enableRL ? '#B8860B' : '#7a6a5a', fontSize: 13 }}>
              RL优化
            </Text>
          </Space>

          <Button
            size="large"
            icon={<SettingOutlined />}
            style={{
              background: 'linear-gradient(135deg, #3a2f28 0%, #2a1f18 100%)',
              border: '1px solid #8B4513',
              color: '#d4c4a8'
            }}
          >
            参数设置
          </Button>
        </Space>
      </div>
    </div>
  )
}

export default SimulationPage
