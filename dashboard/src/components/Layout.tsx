import { NavLink, Outlet } from 'react-router-dom'
import { useAuth } from '../hooks/useAuth'

const navItems = [
  { to: '/realtime',  label: 'Realtime',  icon: '📊' },
  { to: '/sessions',  label: 'Sessions',  icon: '🎮' },
  { to: '/heatmap',   label: 'Heatmap',   icon: '🖱️' },
  { to: '/labels',    label: 'Labels',    icon: '🏷️' },
  { to: '/export',    label: 'Export',    icon: '📥' },
]

export default function Layout() {
  const { user, logout } = useAuth()

  return (
    <div className="flex h-screen overflow-hidden">
      {/* ── Sidebar ─────────────────────────────────────────────────────── */}
      <aside className="w-56 flex-none flex flex-col bg-slate-900 border-r border-slate-700">
        {/* Logo */}
        <div className="px-5 py-4 border-b border-slate-700">
          <span className="text-lg font-bold text-blue-400">Game Monitor</span>
        </div>

        {/* Nav */}
        <nav className="flex-1 px-3 py-4 space-y-1 overflow-y-auto">
          {navItems.map(({ to, label, icon }) => (
            <NavLink
              key={to}
              to={to}
              className={({ isActive }) =>
                `flex items-center gap-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-blue-600 text-white'
                    : 'text-slate-400 hover:bg-slate-800 hover:text-slate-100'
                }`
              }
            >
              <span>{icon}</span>
              {label}
            </NavLink>
          ))}
        </nav>

        {/* User */}
        <div className="px-4 py-3 border-t border-slate-700">
          <p className="text-xs text-slate-500 truncate">{user?.email}</p>
          <button
            onClick={logout}
            className="mt-1 text-xs text-slate-400 hover:text-red-400 transition-colors"
          >
            Sign out
          </button>
        </div>
      </aside>

      {/* ── Main ───────────────────────────────────────────────────────── */}
      <main className="flex-1 flex flex-col overflow-hidden">
        <div className="flex-1 overflow-y-auto p-6">
          <Outlet />
        </div>
      </main>
    </div>
  )
}
