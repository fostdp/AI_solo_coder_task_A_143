/**
 * fire_rate_panel.js
 *
 * 射速优化控制面板
 * 功能：仿真控制、强化学习训练控制、射速监控、RL训练监控、告警展示
 *
 * 依赖：React, Ant Design, ECharts
 */

import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  Card,
  Button,
  Slider,
  Switch,
  Statistic,
  Row,
  Col,
  Progress,
  Tag,
  Alert,
  Space,
  Table,
  Typography,
  Badge,
  Tabs,
  Empty,
} from 'antd';
import {
  PlayCircleOutlined,
  PauseCircleOutlined,
  ReloadOutlined,
  ThunderboltOutlined,
  WarningOutlined,
  CheckCircleOutlined,
  DashboardOutlined,
  LineChartOutlined,
  RocketOutlined,
} from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import { format } from 'date-fns';

const { Title, Text } = Typography;

// ============ 类型定义 ============
export interface FireRatePanelProps {
  crossbowID: string;
  isConnected: boolean;
  sensorData?: SensorData;
  rlStatus?: RLStatus;
  alerts?: AlertData[];
  historyData?: HistoryPoint[];
  onStartSimulation: (speed: number, enableRL: boolean) => void;
  onStopSimulation: () => void;
  onResetSimulation: () => void;
  onStartRLTraining: (magazineCapacity: number) => void;
  onPauseRLTraining: () => void;
  onResumeRLTraining: () => void;
  onResolveAlert: (alertID: string) => void;
}

export interface SensorData {
  stringTension: number;
  armDeformation: number;
  magazinePosition: number;
  fireRate: number;
  timestamp: Date;
}

export interface RLStatus {
  isTraining: boolean;
  episode: number;
  totalReward: number;
  averageReward: number;
  epsilon: number;
  converged: boolean;
  bestReward: number;
}

export interface AlertData {
  id: string;
  type: string;
  level: 'info' | 'warning' | 'danger' | 'critical';
  message: string;
  value: number;
  threshold: number;
  timestamp: Date;
  resolved: boolean;
}

export interface HistoryPoint {
  timestamp: Date;
  fireRate: number;
  tension: number;
  fatigue: number;
}

const DEFAULT_SENSOR_DATA: SensorData = {
  stringTension: 200,
  armDeformation: 0,
  magazinePosition: 0,
  fireRate: 0,
  timestamp: new Date(),
};

const DEFAULT_RL_STATUS: RLStatus = {
  isTraining: false,
  episode: 0,
  totalReward: 0,
  averageReward: 0,
  epsilon: 0,
  converged: false,
  bestReward: 0,
};

// ============ 告警级别配置 ============
const ALERT_LEVEL_CONFIG = {
  info:     { color: 'blue',   icon: <CheckCircleOutlined /> },
  warning:  { color: 'orange', icon: <WarningOutlined /> },
  danger:   { color: 'red',    icon: <ThunderboltOutlined /> },
  critical: { color: 'purple', icon: <WarningOutlined /> },
};

// ============ 子组件：仿真控制卡片 ============
function SimulationControlCard({
  isRunning,
  simulationSpeed,
  enableRL,
  onSpeedChange,
  onEnableRLChange,
  onStart,
  onStop,
  onReset,
}) {
  return (
    <Card
      title={
        <Space>
          <DashboardOutlined />
          <span>仿真控制</span>
          {isRunning && <Badge status="processing" text="运行中" />}
        </Space>
      }
      size="small"
    >
      <Space direction="vertical" style={{ width: '100%' }}>
        <div>
          <Text type="secondary">仿真速度</Text>
          <Slider
            min={0.5}
            max={100}
            step={0.5}
            value={simulationSpeed}
            onChange={onSpeedChange}
            tooltip={{ formatter: v => `${v}x 实时` }}
          />
          <div style={{ textAlign: 'right' }}>
            <Tag color="blue">{simulationSpeed.toFixed(1)}x 实时</Tag>
          </div>
        </div>

        <div>
          <Space>
            <Text type="secondary">启用RL优化</Text>
            <Switch checked={enableRL} onChange={onEnableRLChange} />
          </Space>
        </div>

        <Space>
          {!isRunning ? (
            <Button
              type="primary"
              icon={<PlayCircleOutlined />}
              onClick={() => onStart(simulationSpeed, enableRL)}
            >
              启动仿真
            </Button>
          ) : (
            <Button
              danger
              icon={<PauseCircleOutlined />}
              onClick={onStop}
            >
              停止仿真
            </Button>
          )}
          <Button icon={<ReloadOutlined />} onClick={onReset}>
            重置
          </Button>
        </Space>
      </Space>
    </Card>
  );
}

// ============ 子组件：实时监控卡片 ============
function RealtimeMonitorCard({ sensorData, config }) {
  const tensionPercent = (sensorData.stringTension / config.maxTension) * 100;
  const deformationPercent = Math.abs(sensorData.armDeformation) / config.maxDeformation * 100;
  const magazinePercent = sensorData.magazinePosition * 100;

  const getTensionStatus = () => {
    if (tensionPercent > 95) return 'exception';
    if (tensionPercent > 80) return 'active';
    return 'normal';
  };

  const getDeformationStatus = () => {
    if (deformationPercent > 120) return 'exception';
    if (deformationPercent > 100) return 'active';
    return 'normal';
  };

  return (
    <Card
      title={
        <Space>
          <LineChartOutlined />
          <span>实时监控</span>
        </Space>
      }
      size="small"
    >
      <Row gutter={[16, 16]}>
        <Col span={12}>
          <Statistic
            title="射速"
            value={sensorData.fireRate}
            precision={1}
            suffix="发/分钟"
            valueStyle={{ color: sensorData.fireRate < config.designFireRate * 0.7 ? '#f5222d' : '#52c41a' }}
          />
          <Progress
            percent={(sensorData.fireRate / config.designFireRate) * 100}
            status={sensorData.fireRate < config.designFireRate * 0.7 ? 'exception' : 'normal'}
            size="small"
          />
        </Col>
        <Col span={12}>
          <Statistic
            title="弓弦张力"
            value={sensorData.stringTension}
            precision={0}
            suffix="N"
          />
          <Progress
            percent={Math.min(tensionPercent, 100)}
            status={getTensionStatus()}
            size="small"
          />
        </Col>
        <Col span={12}>
          <Statistic
            title="弩臂变形"
            value={Math.abs(sensorData.armDeformation) * 1000}
            precision={1}
            suffix="mm"
          />
          <Progress
            percent={Math.min(deformationPercent, 100)}
            status={getDeformationStatus()}
            size="small"
          />
        </Col>
        <Col span={12}>
          <Statistic
            title="箭匣位置"
            value={magazinePercent}
            precision={0}
            suffix="%"
          />
          <Progress percent={magazinePercent} size="small" />
        </Col>
      </Row>

      <div style={{ marginTop: 16, textAlign: 'right' }}>
        <Tag color="default">
          更新时间: {format(sensorData.timestamp, 'HH:mm:ss.SSS')}
        </Tag>
      </div>
    </Card>
  );
}

// ============ 子组件：RL训练监控卡片 ============
function RLTrainingCard({
  rlStatus,
  onStartTraining,
  onPauseTraining,
  onResumeTraining,
}) {
  const [magazineCapacity, setMagazineCapacity] = useState(10);
  const rewardHistory = useRef<number[]>([]);

  useEffect(() => {
    if (rlStatus.averageReward > 0) {
      rewardHistory.current.push(rlStatus.averageReward);
      if (rewardHistory.current.length > 50) {
        rewardHistory.current = rewardHistory.current.slice(-50);
      }
    }
  }, [rlStatus.averageReward]);

  const rewardChartOption = {
    tooltip: { trigger: 'axis' },
    grid: { left: 40, right: 20, top: 20, bottom: 30 },
    xAxis: {
      type: 'category',
      data: rewardHistory.current.map((_, i) => i),
      name: '步数',
    },
    yAxis: { type: 'value', name: '平均奖励' },
    series: [
      {
        type: 'line',
        data: rewardHistory.current,
        smooth: true,
        lineStyle: { color: '#1890ff' },
        areaStyle: { color: 'rgba(24, 144, 255, 0.2)' },
      },
    ],
  };

  return (
    <Card
      title={
        <Space>
          <RocketOutlined />
          <span>强化学习训练</span>
          {rlStatus.isTraining && (
            <Badge status="processing" text="训练中" />
          )}
          {rlStatus.converged && (
            <Tag color="green">已收敛</Tag>
          )}
        </Space>
      }
      size="small"
    >
      <Space direction="vertical" style={{ width: '100%' }}>
        <Row gutter={[16, 16]}>
          <Col span={8}>
            <Statistic
              title="当前Episode"
              value={rlStatus.episode}
              valueStyle={{ fontSize: 20 }}
            />
          </Col>
          <Col span={8}>
            <Statistic
              title="平均奖励"
              value={rlStatus.averageReward}
              precision={1}
              valueStyle={{ fontSize: 20 }}
            />
          </Col>
          <Col span={8}>
            <Statistic
              title="最佳奖励"
              value={rlStatus.bestReward}
              precision={1}
              valueStyle={{ fontSize: 20, color: '#52c41a' }}
            />
          </Col>
        </Row>

        <div>
          <Text type="secondary">探索率 ε</Text>
          <Progress
            percent={rlStatus.epsilon * 100}
            format={percent => `${(percent / 100).toFixed(3)}`}
            size="small"
          />
        </div>

        {rewardHistory.current.length > 2 && (
          <div style={{ height: 150 }}>
            <ReactECharts
              option={rewardChartOption}
              style={{ height: '100%' }}
              notMerge
            />
          </div>
        )}

        <Space>
          {!rlStatus.isTraining ? (
            <Button
              type="primary"
              icon={<RocketOutlined />}
              onClick={() => onStartTraining(magazineCapacity)}
            >
              开始训练
            </Button>
          ) : (
            <>
              <Button
                icon={<PauseCircleOutlined />}
                onClick={onPauseTraining}
              >
                暂停
              </Button>
              <Button
                icon={<PlayCircleOutlined />}
                onClick={onResumeTraining}
              >
                恢复
              </Button>
            </>
          )}
          <span>箭匣容量:</span>
          <input
            type="number"
            min="1"
            max="50"
            value={magazineCapacity}
            onChange={e => setMagazineCapacity(parseInt(e.target.value) || 10)}
            style={{ width: 80, padding: '4px 8px', border: '1px solid #d9d9d9', borderRadius: 4 }}
          />
        </Space>
      </Space>
    </Card>
  );
}

// ============ 子组件：历史趋势卡片 ============
function HistoryTrendCard({ historyData, config }) {
  const chartOption = useMemo(() => {
    const times = historyData?.map(p => format(p.timestamp, 'HH:mm:ss')) || [];
    const fireRates = historyData?.map(p => p.fireRate) || [];
    const tensions = historyData?.map(p => p.tension / 100) || [];
    const fatigues = historyData?.map(p => p.fatigue * 100) || [];

    return {
      tooltip: { trigger: 'axis' },
      legend: { data: ['射速(发/分)', '张力(×100N)', '疲劳(%)'] },
      grid: { left: 40, right: 60, top: 40, bottom: 30 },
      xAxis: { type: 'category', data: times },
      yAxis: [
        { type: 'value', name: '射速/张力' },
        { type: 'value', name: '疲劳(%)', max: 100 },
      ],
      series: [
        {
          name: '射速(发/分)',
          type: 'line',
          data: fireRates,
          smooth: true,
          lineStyle: { color: '#52c41a' },
        },
        {
          name: '张力(×100N)',
          type: 'line',
          data: tensions,
          smooth: true,
          lineStyle: { color: '#1890ff' },
        },
        {
          name: '疲劳(%)',
          type: 'line',
          data: fatigues,
          smooth: true,
          yAxisIndex: 1,
          lineStyle: { color: '#faad14' },
        },
      ],
    };
  }, [historyData]);

  return (
    <Card
      title={
        <Space>
          <LineChartOutlined />
          <span>历史趋势</span>
        </Space>
      }
      size="small"
    >
      {historyData && historyData.length > 0 ? (
        <div style={{ height: 250 }}>
          <ReactECharts option={chartOption} style={{ height: '100%' }} notMerge />
        </div>
      ) : (
        <Empty description="暂无数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
      )}
    </Card>
  );
}

// ============ 子组件：告警列表卡片 ============
function AlertsCard({ alerts, onResolveAlert }) {
  const activeAlerts = alerts?.filter(a => !a.resolved) || [];
  const resolvedAlerts = alerts?.filter(a => a.resolved) || [];

  const columns = [
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      render: level => {
        const cfg = ALERT_LEVEL_CONFIG[level];
        return <Tag color={cfg.color}>{cfg.icon} {level}</Tag>;
      },
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: type => {
        const labels = {
          string_tension: '弓弦张力',
          fire_rate: '射速',
          fatigue: '疲劳',
          deformation: '弩臂变形',
          jamming: '卡死风险',
        };
        return labels[type] || type;
      },
    },
    {
      title: '数值/阈值',
      key: 'value',
      render: (_, record) => (
        <span>
          {record.value.toFixed(1)} / {record.threshold.toFixed(1)}
        </span>
      ),
    },
    {
      title: '消息',
      dataIndex: 'message',
      key: 'message',
    },
    {
      title: '时间',
      dataIndex: 'timestamp',
      key: 'timestamp',
      render: t => format(new Date(t), 'HH:mm:ss'),
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        !record.resolved && (
          <Button size="small" onClick={() => onResolveAlert(record.id)}>
            标记已处理
          </Button>
        )
      ),
    },
  ];

  const tabItems = [
    {
      key: 'active',
      label: (
        <Space>
          活跃告警
          {activeAlerts.length > 0 && <Badge count={activeAlerts.length} color="red" />}
        </Space>
      ),
      children: (
        <Table
          dataSource={activeAlerts}
          columns={columns}
          rowKey="id"
          size="small"
          pagination={false}
          locale={{ emptyText: '暂无活跃告警' }}
        />
      ),
    },
    {
      key: 'resolved',
      label: `已处理 (${resolvedAlerts.length})`,
      children: (
        <Table
          dataSource={resolvedAlerts}
          columns={columns}
          rowKey="id"
          size="small"
          pagination={{ pageSize: 5 }}
        />
      ),
    },
  ];

  return (
    <Card
      title={
        <Space>
          <WarningOutlined />
          <span>告警中心</span>
          {activeAlerts.length > 0 && (
            <Badge count={activeAlerts.length} color="red" />
          )}
        </Space>
      }
      size="small"
    >
      <Tabs items={tabItems} defaultActiveKey="active" size="small" />
    </Card>
  );
}

// ============ 主组件：射速优化控制面板 ============
export function FireRatePanel({
  crossbowID,
  isConnected,
  sensorData = DEFAULT_SENSOR_DATA,
  rlStatus = DEFAULT_RL_STATUS,
  alerts = [],
  historyData = [],
  onStartSimulation,
  onStopSimulation,
  onResetSimulation,
  onStartRLTraining,
  onPauseRLTraining,
  onResumeRLTraining,
  onResolveAlert,
}: FireRatePanelProps) {
  const [simulationSpeed, setSimulationSpeed] = useState(1);
  const [enableRL, setEnableRL] = useState(false);
  const [isRunning, setIsRunning] = useState(false);

  const config = {
    maxTension: 1500,
    designFireRate: 10,
    maxDeformation: 0.02,
  };

  const handleStart = useCallback((speed, rl) => {
    setIsRunning(true);
    onStartSimulation(speed, rl);
  }, [onStartSimulation]);

  const handleStop = useCallback(() => {
    setIsRunning(false);
    onStopSimulation();
  }, [onStopSimulation]);

  const handleReset = useCallback(() => {
    setIsRunning(false);
    onResetSimulation();
  }, [onResetSimulation]);

  // 最新活跃告警用于顶部显示
  const latestCriticalAlert = alerts
    .filter(a => !a.resolved && (a.level === 'critical' || a.level === 'danger'))
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime())[0];

  return (
    <div style={{ padding: 16, background: '#f0f2f5', minHeight: '100vh' }}>
      {/* 标题栏 */}
      <div style={{ marginBottom: 16 }}>
        <Space>
          <Title level={4} style={{ margin: 0 }}>
            诸葛连弩射速优化系统
          </Title>
          <Tag color={isConnected ? 'green' : 'red'}>
            {isConnected ? 'WebSocket已连接' : 'WebSocket未连接'}
          </Tag>
          <Tag color="blue">ID: {crossbowID}</Tag>
        </Space>
      </div>

      {/* 紧急告警横幅 */}
      {latestCriticalAlert && (
        <Alert
          message={latestCriticalAlert.message}
          type={latestCriticalAlert.level === 'critical' ? 'error' : 'warning'}
          showIcon
          closable
          style={{ marginBottom: 16 }}
          action={
            <Button size="small" type="primary" danger onClick={() => onResolveAlert(latestCriticalAlert.id)}>
              立即处理
            </Button>
          }
        />
      )}

      {/* 主要内容区 */}
      <Row gutter={[16, 16]}>
        <Col xs={24} lg={12}>
          <Row gutter={[16, 16]}>
            <Col span={24}>
              <SimulationControlCard
                isRunning={isRunning}
                simulationSpeed={simulationSpeed}
                enableRL={enableRL}
                onSpeedChange={setSimulationSpeed}
                onEnableRLChange={setEnableRL}
                onStart={handleStart}
                onStop={handleStop}
                onReset={handleReset}
              />
            </Col>
            <Col span={24}>
              <RealtimeMonitorCard sensorData={sensorData} config={config} />
            </Col>
          </Row>
        </Col>

        <Col xs={24} lg={12}>
          <Row gutter={[16, 16]}>
            <Col span={24}>
              <RLTrainingCard
                rlStatus={rlStatus}
                onStartTraining={onStartRLTraining}
                onPauseTraining={onPauseRLTraining}
                onResumeTraining={onResumeRLTraining}
              />
            </Col>
          </Row>
        </Col>

        <Col span={24}>
          <HistoryTrendCard historyData={historyData} config={config} />
        </Col>

        <Col span={24}>
          <AlertsCard alerts={alerts} onResolveAlert={onResolveAlert} />
        </Col>
      </Row>

      {/* 页脚 */}
      <div style={{ marginTop: 24, textAlign: 'center', color: '#999' }}>
        <Text type="secondary">
          古代连弩（诸葛弩）机构动力学仿真与射速优化系统 v1.0
        </Text>
      </div>
    </div>
  );
}

export default FireRatePanel;
