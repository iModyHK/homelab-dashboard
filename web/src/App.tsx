import { Routes, Route } from 'react-router-dom'

function Placeholder({ title }: { title: string }) {
  return (
    <main className="mx-auto max-w-7xl p-6">
      <h1 className="text-2xl font-semibold">{title}</h1>
    </main>
  )
}

export default function App() {
  return (
    <div className="dark min-h-full">
      <Routes>
        <Route path="/" element={<Placeholder title="Homelab Dashboard" />} />
      </Routes>
    </div>
  )
}
