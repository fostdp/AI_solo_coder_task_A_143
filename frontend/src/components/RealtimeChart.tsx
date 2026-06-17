import React, { useMemo } from 'react'
import ReactECharts from 'echarts-for-react'
import type { EChartsOption } from 'echarts'

interface DataPoint {
  time: string | number
  [key: string]: string | number
}

interface SeriesConfig {
  key: string
  name: string
  color: string
  unit?: string
}

interface RealtimeChartProps {
  data: DataPoint[]
  series: SeriesConfig[]
  title?: string
  xAxisKey?: string
  maxPoints?: number
  height?: number | string
  showLegend?: boolean
  thresholdLines?: Array<{
    value: number
    name: string
    color: string
  }>
}

const RealtimeChart: React.FC<RealtimeChartProps> = ({
  data,
  series,
  title,
  xAxisKey = 'time',
  maxPoints = 50,
  height = 200,
  showLegend = true,
  thresholdLines = []
}) => {
  const displayData = useMemo(() => {
    return data.slice(-maxPoints)
  }, [data, maxPoints])

  const option: EChartsOption = useMemo(() => {
    const seriesConfig = series.map(s => ({
      name: s.name,
      type: 'line',
      smooth: true,
      symbol: 'circle',
      symbolSize: 6,
      showSymbol: false,
      lineStyle: {
        width: 2,
        color: s.color
      },
      itemStyle: {
        color: s.color
      },
      areaStyle: {
        color: {
          type: 'linear',
          x: 0,
          y: 0,
          x2: 0,
          y2: 1,
          colorStops: [
            { offset: 0, color: `${s.color}40` },
            { offset: 1, color: `${s.color}05` }
          ]
        }
      },
      data: displayData.map(d => d[s.key] as number)
    }))

    const markLines = thresholdLines.length > 0 ? {
      silent: true,
      symbol: 'none',
      data: thresholdLines.map(t => ({
        yAxis: t.value,
        label: {
          formatter: t.name,
          position: 'end',
          color: t.color,
          fontSize: 11
        },
        lineStyle: {
          color: t.color,
          type: 'dashed',
          width: 1
        }
      }))
    } : undefined

    return {
      backgroundColor: 'transparent',
      title: title ? {
        text: title,
        textStyle: {
          color: '#B8860B',
          fontSize: 14,
          fontFamily: "'Noto Serif SC', serif",
          fontWeight: 500
        },
        left: 'center',
        top: 5
      } : undefined,
      tooltip: {
        trigger: 'axis',
        backgroundColor: 'rgba(26, 20, 16, 0.95)',
        borderColor: '#8B4513',
        borderWidth: 1,
        textStyle: {
          color: '#d4c4a8',
          fontFamily: "'Noto Serif SC', serif"
        },
        axisPointer: {
          type: 'cross',
          lineStyle: {
            color: '#B8860B',
            type: 'dashed'
          }
        },
        formatter: (params: unknown) => {
          const p = params as Array<{ axisValue: string; seriesName: string; value: number; marker: string }>
          if (!p.length) return ''
          let html = `<div style="margin-bottom: 4px; color: #a89888; font-size: 12px;">${p[0].axisValue}</div>`
          p.forEach(item => {
            const seriesConfig = series.find(s => s.name === item.seriesName)
            const unit = seriesConfig?.unit || ''
            html += `<div style="display: flex; align-items: center; gap: 8px; margin: 4px 0;">
              ${item.marker}
              <span style="color: #d4c4a8;">${item.seriesName}:</span>
              <span style="color: #B8860B; font-weight: bold;">${item.value.toFixed(2)}${unit ? ' ' + unit : ''}</span>
            </div>`
          })
          return html
        }
      },
      legend: showLegend ? {
        show: true,
        top: title ? 30 : 5,
        textStyle: {
          color: '#a89888',
          fontSize: 12,
          fontFamily: "'Noto Serif SC', serif"
        },
        itemGap: 16,
        itemWidth: 20,
        itemHeight: 2
      } : undefined,
      grid: {
        left: '3%',
        right: '4%',
        bottom: '3%',
        top: showLegend ? (title ? 60 : 35) : (title ? 40 : 10),
        containLabel: true
      },
      xAxis: {
        type: 'category',
        boundaryGap: false,
        data: displayData.map(d => d[xAxisKey] as string),
        axisLine: {
          lineStyle: {
            color: '#5a4a3a'
          }
        },
        axisTick: {
          show: false
        },
        axisLabel: {
          color: '#7a6a5a',
          fontSize: 10,
          fontFamily: "'Noto Serif SC', serif",
          rotate: 0,
          formatter: (value: string) => {
            if (typeof value === 'string' && value.includes('T')) {
              return value.split('T')[1]?.slice(0, 8) || value
            }
            return value
          }
        },
        splitLine: {
          show: false
        }
      },
      yAxis: {
        type: 'value',
        axisLine: {
          show: false
        },
        axisTick: {
          show: false
        },
        axisLabel: {
          color: '#7a6a5a',
          fontSize: 10,
          fontFamily: "'Noto Serif SC', serif"
        },
        splitLine: {
          lineStyle: {
            color: '#3a2f28',
            type: 'dashed'
          }
        }
      },
      series: seriesConfig.map(s => ({
        ...s,
        markLine: markLines
      }))
    }
  }, [displayData, series, title, xAxisKey, showLegend, thresholdLines])

  return (
    <div
      style={{
        background: 'linear-gradient(180deg, #2a1f18 0%, #1a1410 100%)',
        border: '1px solid #8B4513',
        borderRadius: 6,
        height,
        padding: '8px 12px'
      }}
    >
      <ReactECharts
        option={option}
        style={{ height: '100%', width: '100%' }}
        opts={{ renderer: 'canvas' }}
      />
    </div>
  )
}

export default RealtimeChart
