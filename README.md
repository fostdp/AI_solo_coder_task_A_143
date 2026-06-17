# 古代连弩（诸葛弩）机构动力学仿真与射速优化系统

为军事史研究团队开发的三国时期诸葛连弩数字化复原研究平台。通过多刚体动力学和凸轮机构模型模拟连弩自动装填与击发过程，结合强化学习优化射击节奏。

## 系统架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                        前端 (React + Three.js)                       │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ 3D仿真   │  │ 实时监控 │  │ 告警系统 │  │ 数据分析 │          │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘          │
└─────────────────────────────────────┬───────────────────────────────┘
                                      │ WebSocket/REST API
┌─────────────────────────────────────▼───────────────────────────────┐
│                        后端 (Go + Gin)                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌──────────┐          │
│  │ 动力学   │  │ 强化学习 │  │ 告警服务 │  │ WebSocket│          │
│  │ 仿真引擎 │  │ 优化器   │  │         │  │ 服务    │          │
│  └──────────┘  └──────────┘  └──────────┘  └──────────┘          │
└─────────────────────────────────────┬───────────────────────────────┘
                                      │
┌─────────────────────────────────────▼───────────────────────────────┐
│                        数据层                                       │
│  ┌──────────────────┐  ┌──────────┐  ┌──────────────────┐          │
│  │ TimescaleDB      │  │  Redis   │  │ 传感器模拟器     │          │
│  │ (时序数据)       │  │ (缓存)   │  │ (Python)         │          │
│  └──────────────────┘  └──────────┘  └──────────────────┘          │
└─────────────────────────────────────────────────────────────────────┘
```

## 核心功能

### 1. 机构动力学仿真
- **多刚体动力学**：基于欧拉-拉格朗日方程求解弩臂运动
- **凸轮机构**：正弦加速度运动规律，等径凸轮设计
- **棘爪棘轮**：状态机模拟自动装填机构
- **接触力学**：Hertz接触理论 + 库仑摩擦
- **数值积分**：四阶龙格-库塔法（RK4）

### 2. 射速优化（强化学习）
- **DQN算法**：深度Q网络，线性函数近似
- **状态空间**：[射速, 疲劳度, 箭匣余量, 平均张力, 射击次数]
- **动作空间**：5种装填间隔调整策略
- **奖励函数**：平衡射速与疲劳累积
- **经验回放** + **目标网络** 保证训练稳定

### 3. 实时告警系统
- 弓弦断裂风险预警
- 射速低于设计值告警
- 疲劳累积预警
- WebSocket实时推送
- 告警冷却机制

### 4. 3D可视化
- Three.js 构建连弩数字模型
- 弓弦动态拉伸动画
- 弩臂弯曲变形模拟
- 箭矢抛物线弹道
- 凸轮机构运动演示

## 快速开始

### 方式一：Docker Compose（推荐）

```bash
# 启动所有服务
docker-compose up -d

# 访问前端
http://localhost:3000

# 查看日志
docker-compose logs -f backend
docker-compose logs -f sensor-simulator
```

### 方式二：本地开发

#### 1. 启动数据库

```bash
# 启动 TimescaleDB
docker run -d --name timescale \
  -e POSTGRES_DB=crossbow_db \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  timescale/timescaledb:2.13.0-pg16

# 启动 Redis
docker run -d --name redis -p 6379:6379 redis:7-alpine

# 初始化数据库
psql -h localhost -U postgres -d crossbow_db -f database/init.sql
```

#### 2. 启动后端

```bash
cd backend

# 安装依赖
go mod download

# 运行
go run ./cmd/server

# 或编译后运行
go build -o server ./cmd/server
./server
```

#### 3. 启动前端

```bash
cd frontend

# 安装依赖
npm install

# 开发模式
npm run dev

# 生产构建
npm run build
```

#### 4. 启动传感器模拟器

```bash
cd sensor-simulator

# 安装依赖
pip install -r requirements.txt

# 正常模式（每60秒上报一次）
python simulator.py

# 疲劳模式
python simulator.py --mode fatigue

# 自定义上报间隔
python simulator.py --report-interval 30
```

## 传感器模拟器故障模式

| 模式 | 描述 |
|------|------|
| `normal` | 正常工作模式，数据在正常范围内波动 |
| `fatigue` | 疲劳模式，射速逐渐降低，疲劳累积加快3倍 |
| `jammed` | 卡弹模式，所有参数卡住不再变化 |
| `string_break` | 断弦模式，张力骤降为0，触发告警 |

## API 接口

### 连弩管理

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/crossbows` | 获取连弩列表 |
| GET | `/api/v1/crossbows/:id` | 获取连弩详情 |
| POST | `/api/v1/crossbows` | 创建连弩 |
| PUT | `/api/v1/crossbows/:id` | 更新连弩配置 |
| POST | `/api/v1/crossbows/:id/start` | 启动仿真 |
| POST | `/api/v1/crossbows/:id/stop` | 停止仿真 |
| POST | `/api/v1/crossbows/:id/reset` | 重置仿真 |

### 数据接口

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/sensor/data` | 传感器数据上报 |
| POST | `/api/v1/data/query` | 时序数据查询 |

### 告警接口

| 方法 | 路径 | 描述 |
|------|------|------|
| GET | `/api/v1/alerts` | 获取告警列表 |
| POST | `/api/v1/alerts/:id/ack` | 确认告警 |
| GET | `/api/v1/alerts/thresholds/:id` | 获取告警阈值 |
| PUT | `/api/v1/alerts/thresholds` | 更新告警阈值 |

### 强化学习接口

| 方法 | 路径 | 描述 |
|------|------|------|
| POST | `/api/v1/rl/train/:id` | 启动训练 |
| GET | `/api/v1/rl/status/:id` | 获取训练状态 |
| GET | `/api/v1/rl/result/:id` | 获取优化结果 |
| POST | `/api/v1/rl/pause` | 暂停训练 |
| POST | `/api/v1/rl/resume` | 恢复训练 |

### WebSocket

```
ws://localhost:8080/ws/crossbow/:id
```

消息类型：
- `sensor_data` - 实时传感器数据
- `dynamics_state` - 动力学状态
- `trajectory` - 箭矢弹道
- `alert` - 告警通知
- `rl_update` - 强化学习更新
- `status` - 状态变更

## 物理模型

### 多刚体动力学方程

```
I·θ̈ + c·θ̇ + k_t·θ + T·L·cos(θ) + mg·L_cm·sin(θ) = M_external
```

其中：
- `I` - 转动惯量
- `c` - 阻尼系数
- `k_t` - 扭转刚度
- `T` - 弓弦张力
- `L` - 弩臂长度

### 凸轮运动规律（正弦加速度）

```
s(φ) = h·[φ/Φ - (1/2π)·sin(2πφ/Φ)]
v(φ) = ω·ds/dφ
a(φ) = ω²·d²s/dφ²
```

### 弓弦张力（非线性胡克定律）

```
T(ΔL) = k·ΔL + α·k·(ΔL)³,  ΔL > 0
T(ΔL) = 0,                  ΔL ≤ 0
```

### 箭矢弹道（考虑空气阻力）

```
ẍ = -k_drag·vx·|v|
ÿ = -g - k_drag·vy·|v|
z̈ = -k_drag·vz·|v|
```

### 疲劳累积（Miner法则 + Basquin方程）

```
D = Σ(ni / Ni)
Ni = (C / σ_aeq^m)^(1/m)
```

## 项目结构

```
project-root/
├── backend/                    # Go后端
│   ├── cmd/server/            # 主程序入口
│   ├── internal/
│   │   ├── api/               # API控制器
│   │   ├── simulation/        # 动力学仿真引擎
│   │   ├── rl/                # 强化学习优化
│   │   ├── alert/             # 告警服务
│   │   ├── websocket/         # WebSocket服务
│   │   ├── repository/        # 数据访问层
│   │   └── model/             # 数据模型
│   ├── config/                # 配置文件
│   ├── Dockerfile
│   └── go.mod
├── frontend/                  # 前端应用
│   ├── src/
│   │   ├── components/        # 通用组件
│   │   ├── pages/             # 页面组件
│   │   ├── three/             # Three.js 3D模型
│   │   ├── store/             # Zustand状态管理
│   │   ├── services/          # API和WebSocket服务
│   │   └── types/             # TypeScript类型
│   ├── Dockerfile
│   ├── nginx.conf
│   └── package.json
├── sensor-simulator/          # 传感器模拟器
│   ├── simulator.py
│   ├── config.yaml
│   └── requirements.txt
├── database/                  # 数据库脚本
│   └── init.sql
├── .trae/documents/           # 设计文档
│   ├── PRD.md
│   └── TECHNICAL_ARCHITECTURE.md
├── docker-compose.yml
└── README.md
```

## 配置说明

### 后端配置 (backend/config/config.yaml)

```yaml
server:
  host: "0.0.0.0"
  port: 8080

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "postgres"
  dbname: "crossbow_db"

redis:
  host: "localhost"
  port: 6379

simulation:
  time_step: 0.01        # 仿真时间步长（秒）
  speed_multiplier: 1.0  # 仿真速度倍率

alert:
  check_interval: 5      # 告警检查间隔（秒）
  cooldown_period: 60    # 告警冷却期（秒）
```

### 告警阈值

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `stringTensionMax` | 1200 N | 弓弦最大张力阈值 |
| `stringFatigueWarning` | 0.7 | 疲劳预警阈值 |
| `fireRateMin` | 6 发/分钟 | 最低射速阈值 |
| `deformationMax` | 20 mm | 最大变形阈值 |

## 技术栈

### 后端
- **Go 1.21** - 高性能编程语言
- **Gin 1.9** - Web框架
- **pgx/v5** - PostgreSQL驱动
- **gorilla/websocket** - WebSocket支持
- **gonum** - 数值计算库
- **TimescaleDB 2.13** - 时序数据库
- **Redis 7** - 缓存和消息队列

### 前端
- **React 18** - UI框架
- **TypeScript 5** - 类型安全
- **Three.js 0.160** - 3D渲染
- **@react-three/fiber** - React Three.js绑定
- **ECharts 5** - 数据可视化
- **Zustand 4** - 状态管理
- **Ant Design 5** - UI组件库
- **Vite 5** - 构建工具

### 传感器模拟器
- **Python 3.11**
- **NumPy 1.26** - 数值计算
- **PyYAML** - 配置解析

## 性能指标

- 仿真频率：100Hz（时间步长10ms）
- 数据上报频率：每60秒1次
- WebSocket延迟：<100ms
- 3D渲染帧率：目标60fps
- 数据库写入：支持1000+点/秒
- 强化学习训练：约1000episode收敛

## 扩展阅读

完整的产品需求文档和技术架构设计请查看：
- [PRD文档](.trae/documents/PRD.md)
- [技术架构文档](.trae/documents/TECHNICAL_ARCHITECTURE.md)

## License

MIT License
