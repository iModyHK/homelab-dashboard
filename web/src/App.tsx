import { Navigate, Route, Routes, useLocation } from 'react-router-dom'
import { Layout } from './components/Layout'
import { Spinner } from './components/ui'
import { useTheme } from './lib/theme'
import { Alerts } from './pages/Alerts'
import { ContainerDetail } from './pages/ContainerDetail'
import { Disks } from './pages/Disks'
import { Errors } from './pages/Errors'
import { Login } from './pages/Login'
import { LogSearch } from './pages/LogSearch'
import { Overview } from './pages/Overview'
import { StackDetail } from './pages/StackDetail'
import { Updates } from './pages/Updates'
import { DashboardProvider, useDashboard } from './store/DashboardProvider'

function Shell() {
  const { authenticated } = useDashboard()
  const [theme, toggleTheme] = useTheme()
  const location = useLocation()

  if (authenticated === null) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <Spinner />
      </div>
    )
  }
  if (!authenticated) {
    return (
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="*" element={<Navigate to="/login" replace state={{ from: location.pathname }} />} />
      </Routes>
    )
  }
  return (
    <Layout theme={theme} toggleTheme={toggleTheme}>
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/stacks/:name" element={<StackDetail />} />
        <Route path="/containers/:id" element={<ContainerDetail theme={theme} />} />
        <Route path="/errors" element={<Errors />} />
        <Route path="/disks" element={<Disks theme={theme} />} />
        <Route path="/updates" element={<Updates />} />
        <Route path="/alerts" element={<Alerts />} />
        <Route path="/search" element={<LogSearch />} />
        <Route path="/login" element={<Navigate to="/" replace />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Layout>
  )
}

export default function App() {
  return (
    <DashboardProvider>
      <Shell />
    </DashboardProvider>
  )
}
