import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { ConfigProvider, theme } from 'antd'
import zhCN from 'antd/locale/zh_CN'

const root = ReactDOM.createRoot(document.getElementById('root')!)

root.render(
  <React.StrictMode>
    <ConfigProvider
      locale={zhCN}
      theme={{
        algorithm: theme.darkAlgorithm,
        token: {
          colorPrimary: '#B8860B',
          colorInfo: '#B8860B',
          colorSuccess: '#2E8B57',
          colorWarning: '#CD853F',
          colorError: '#8B0000',
          colorBgBase: '#1a1410',
          colorBgContainer: '#2a1f18',
          colorBgElevated: '#3a2f28',
          colorBorder: '#8B4513',
          colorBorderSecondary: '#5a4a3a',
          colorText: '#d4c4a8',
          colorTextSecondary: '#a89888',
          colorTextPlaceholder: '#7a6a5a',
          fontFamily: "'Noto Serif SC', serif",
          fontSize: 14,
          borderRadius: 4
        },
        components: {
          Layout: {
            headerBg: '#2a1f18',
            bodyBg: '#1a1410',
            siderBg: '#2a1f18',
            footerBg: '#2a1f18'
          },
          Menu: {
            darkItemBg: 'transparent',
            darkSubMenuItemBg: 'transparent',
            darkItemSelectedBg: '#8B4513',
            darkItemSelectedColor: '#FFD700',
            darkItemHoverBg: '#3a2f28',
            darkItemColor: '#d4c4a8'
          },
          Button: {
            colorPrimary: '#B8860B',
            colorPrimaryHover: '#DAA520',
            colorPrimaryActive: '#8B6914',
            defaultBg: '#3a2f28',
            defaultBorderColor: '#8B4513',
            defaultColor: '#d4c4a8',
            fontWeight: 500
          }
        }
      }}
    >
      <App />
    </ConfigProvider>
  </React.StrictMode>
)
