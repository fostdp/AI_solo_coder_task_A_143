import React, { useMemo } from 'react'

interface GaugeProps {
  value: number
  min?: number
  max?: number
  title: string
  unit?: string
  warningThreshold?: number
  dangerThreshold?: number
  size?: number
}

const Gauge: React.FC<GaugeProps> = ({
  value,
  min = 0,
  max = 100,
  title,
  unit = '',
  warningThreshold,
  dangerThreshold,
  size = 180
}) => {
  const radius = size / 2
  const strokeWidth = 12
  const innerRadius = radius - strokeWidth - 10
  const startAngle = -135
  const endAngle = 135
  const angleRange = endAngle - startAngle

  const clampedValue = Math.max(min, Math.min(max, value))
  const percentage = (clampedValue - min) / (max - min)
  const angle = startAngle + percentage * angleRange

  const polarToCartesian = (cx: number, cy: number, r: number, angleDeg: number) => {
    const angleRad = (angleDeg - 90) * (Math.PI / 180)
    return {
      x: cx + r * Math.cos(angleRad),
      y: cy + r * Math.sin(angleRad)
    }
  }

  const describeArc = (cx: number, cy: number, r: number, startAngle: number, endAngle: number) => {
    const start = polarToCartesian(cx, cy, r, endAngle)
    const end = polarToCartesian(cx, cy, r, startAngle)
    const largeArcFlag = endAngle - startAngle <= 180 ? '0' : '1'
    return `M ${start.x} ${start.y} A ${r} ${r} 0 ${largeArcFlag} 0 ${end.x} ${end.y}`
  }

  const getValueColor = () => {
    if (dangerThreshold !== undefined && clampedValue >= dangerThreshold) return '#8B0000'
    if (warningThreshold !== undefined && clampedValue >= warningThreshold) return '#CD853F'
    return '#B8860B'
  }

  const getGlowColor = () => {
    if (dangerThreshold !== undefined && clampedValue >= dangerThreshold) return 'rgba(139, 0, 0, 0.5)'
    if (warningThreshold !== undefined && clampedValue >= warningThreshold) return 'rgba(205, 133, 63, 0.5)'
    return 'rgba(184, 134, 11, 0.5)'
  }

  const ticks = useMemo(() => {
    const tickCount = 11
    const result = []
    for (let i = 0; i < tickCount; i++) {
      const tickPercentage = i / (tickCount - 1)
      const tickAngle = startAngle + tickPercentage * angleRange
      const tickValue = min + tickPercentage * (max - min)
      const outerPoint = polarToCartesian(radius, radius, innerRadius + 2, tickAngle)
      const innerPoint = polarToCartesian(radius, radius, innerRadius - 8, tickAngle)
      const labelPoint = polarToCartesian(radius, radius, innerRadius - 20, tickAngle)
      result.push({ tickAngle, tickValue, outerPoint, innerPoint, labelPoint })
    }
    return result
  }, [startAngle, angleRange, min, max, innerRadius, radius])

  const backgroundArc = describeArc(radius, radius, innerRadius, startAngle, endAngle)

  const createColoredArc = (startPct: number, endPct: number, color: string) => {
    const arcStart = startAngle + startPct * angleRange
    const arcEnd = startAngle + endPct * angleRange
    return {
      path: describeArc(radius, radius, innerRadius, arcStart, arcEnd),
      color
    }
  }

  const coloredArcs = useMemo(() => {
    const arcs = []
    let currentPct = 0

    if (warningThreshold !== undefined) {
      const warnPct = (warningThreshold - min) / (max - min)
      if (warnPct > currentPct) {
        arcs.push(createColoredArc(currentPct, warnPct, '#2E8B57'))
        currentPct = warnPct
      }
    }

    if (dangerThreshold !== undefined) {
      const dangerPct = (dangerThreshold - min) / (max - min)
      if (dangerPct > currentPct) {
        arcs.push(createColoredArc(currentPct, dangerPct, '#CD853F'))
        currentPct = dangerPct
      }
    }

    if (currentPct < 1) {
      arcs.push(createColoredArc(currentPct, 1, '#8B0000'))
    }

    return arcs
  }, [startAngle, angleRange, min, max, warningThreshold, dangerThreshold, innerRadius, radius])

  const needleEnd = polarToCartesian(radius, radius, innerRadius - 15, angle)

  const displayValue = value.toFixed(value % 1 === 0 ? 0 : 1)

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
      <div
        style={{
          position: 'relative',
          width: size,
          height: size
        }}
      >
        <svg width={size} height={size} style={{ filter: `drop-shadow(0 0 10px ${getGlowColor()})` }}>
          <defs>
            <radialGradient id={`gaugeBg-${title}`} cx="50%" cy="50%" r="50%">
              <stop offset="0%" stopColor="#3a2f28" />
              <stop offset="70%" stopColor="#2a1f18" />
              <stop offset="100%" stopColor="#1a1410" />
            </radialGradient>
            <linearGradient id={`gaugeRing-${title}`} x1="0%" y1="0%" x2="100%" y2="100%">
              <stop offset="0%" stopColor="#8B4513" />
              <stop offset="50%" stopColor="#B8860B" />
              <stop offset="100%" stopColor="#8B4513" />
            </linearGradient>
            <filter id={`glow-${title}`}>
              <feGaussianBlur stdDeviation="2" result="coloredBlur" />
              <feMerge>
                <feMergeNode in="coloredBlur" />
                <feMergeNode in="SourceGraphic" />
              </feMerge>
            </filter>
          </defs>

          <circle cx={radius} cy={radius} r={radius - 2} fill={`url(#gaugeBg-${title})`} />
          <circle cx={radius} cy={radius} r={radius - 2} fill="none" stroke={`url(#gaugeRing-${title})`} strokeWidth="3" />

          <path
            d={backgroundArc}
            fill="none"
            stroke="#3a3a2a"
            strokeWidth={strokeWidth}
            strokeLinecap="round"
            opacity={0.5}
          />

          {coloredArcs.map((arc, index) => (
            <path
              key={index}
              d={arc.path}
              fill="none"
              stroke={arc.color}
              strokeWidth={strokeWidth}
              strokeLinecap="round"
              opacity={0.6}
            />
          ))}

          {ticks.map((tick, i) => (
            <g key={i}>
              <line
                x1={tick.outerPoint.x}
                y1={tick.outerPoint.y}
                x2={tick.innerPoint.x}
                y2={tick.innerPoint.y}
                stroke={i % 2 === 0 ? '#B8860B' : '#8B4513'}
                strokeWidth={i % 2 === 0 ? 2 : 1}
              />
              <text
                x={tick.labelPoint.x}
                y={tick.labelPoint.y}
                fill="#a89888"
                fontSize="10"
                textAnchor="middle"
                dominantBaseline="middle"
                fontFamily="'Noto Serif SC', serif"
              >
                {tick.tickValue.toFixed(tick.tickValue % 1 === 0 ? 0 : 0)}
              </text>
            </g>
          ))}

          <g filter={`url(#glow-${title})`}>
            <line
              x1={radius}
              y1={radius}
              x2={needleEnd.x}
              y2={needleEnd.y}
              stroke={getValueColor()}
              strokeWidth="3"
              strokeLinecap="round"
              style={{
                transition: 'transform 0.3s ease-out',
                transformOrigin: `${radius}px ${radius}px`
              }}
            />
          </g>

          <circle cx={radius} cy={radius} r="8" fill={`url(#gaugeRing-${title})`} />
          <circle cx={radius} cy={radius} r="4" fill="#1a1410" />
        </svg>

        <div
          style={{
            position: 'absolute',
            bottom: size * 0.2,
            left: '50%',
            transform: 'translateX(-50%)',
            textAlign: 'center'
          }}
        >
          <div
            style={{
              fontSize: size * 0.18,
              fontWeight: 'bold',
              color: getValueColor(),
              fontFamily: "'Noto Serif SC', serif",
              textShadow: `0 0 10px ${getGlowColor()}`,
              transition: 'color 0.3s ease'
            }}
          >
            {displayValue}
            <span style={{ fontSize: size * 0.1, marginLeft: 4, color: '#a89888' }}>{unit}</span>
          </div>
        </div>
      </div>

      <div
        style={{
          marginTop: 8,
          fontSize: 14,
          color: '#d4c4a8',
          fontFamily: "'Noto Serif SC', serif",
          letterSpacing: 2
        }}
      >
        {title}
      </div>
    </div>
  )
}

export default Gauge
