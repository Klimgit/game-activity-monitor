import { Navigate, Route, Routes } from 'react-router-dom'
import { useAuth } from './hooks/useAuth'
import Layout from './components/Layout'
import Login from './pages/Login'
import Realtime from './pages/Realtime'
import Sessions from './pages/Sessions'
import Heatmap from './pages/Heatmap'
import Export from './pages/Export'

function ProtectedRoute({ children }: { children: React.ReactNode }) {
  const { isAuthenticated } = useAuth()
  return isAuthenticated ? <>{children}</> : <Navigate to="/login" replace />
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />

      <Route
        element={
          <ProtectedRoute>
            <Layout />
          </ProtectedRoute>
        }
      >
        <Route index element={<Navigate to="/realtime" replace />} />
        <Route path="/realtime" element={<Realtime />} />
        <Route path="/sessions" element={<Sessions />} />
        <Route path="/heatmap" element={<Heatmap />} />
        <Route path="/export" element={<Export />} />
      </Route>
    </Routes>
  )
}
