export interface Live {
  ts: number
  cpu: number
  mem: number
  memLimit: number
  netRx: number
  netTx: number
  blkRead: number
  blkWrite: number
  pids: number
}

export interface PortMapping {
  hostIp: string
  hostPort: number
  containerPort: number
  protocol: string
}

export interface MountInfo {
  type: string
  source: string
  destination: string
  readOnly: boolean
}

export interface HealthCheck {
  start: number
  exitCode: number
  output: string
}

export interface Container {
  id: string
  name: string
  image: string
  stack: string
  stackSource: string
  service: string
  state: string
  status: string
  health: string
  restartCount: number
  restartPolicy: string
  created: number
  startedAt: number
  finishedAt: number
  exitCode: number
  memoryLimit: number
  cpuLimit: number
  updateAvailable: boolean
  live: Live | null
  sparkline: number[]
  ports: PortMapping[]
}

export interface ContainerDetail extends Container {
  imageId: string
  imageDigest: string
  endpointId: number
  failingStreak: number
  healthLog: HealthCheck[] | null
  oomKilled: boolean
  error: string
  tty: boolean
  mounts: MountInfo[] | null
  env: string[] | null
  labels: Record<string, string> | null
  networks: Record<string, string> | null
}

export interface StackSummary {
  name: string
  source: string
  endpointId: number
  total: number
  running: number
  unhealthy: number
  exited: number
  cpu: number
  mem: number
  netRx: number
  netTx: number
  updates: number
  health: string
  portainerId: number
  stackType: string
  stackStatus: string
}

export interface MountUsage {
  Mount: string
  Used: number
  Total: number
  Free: number
}

export interface HostState {
  ts: number
  hostname: string
  os: string
  kernel: string
  dockerVersion: string
  cpus: number
  cpu: number
  cpuTemp: number
  load1: number
  load5: number
  load15: number
  memUsed: number
  memTotal: number
  swapUsed: number
  swapTotal: number
  netRx: number
  netTx: number
  uptime: number
  mounts: MountUsage[] | null
}

export interface EndpointSummary {
  id: number
  name: string
  online: boolean
}

export interface PortainerState {
  version: string
  endpoints: EndpointSummary[] | null
}

export interface Disk {
  device: string
  model: string
  serial: string
  firmware: string
  capacityBytes: number
  rotationRpm: number
  transport: string
  temperature: number
  powerOnHours: number
  powerCycles: number
  reallocated: number
  pending: number
  uncorrectable: number
  crcErrors: number
  smartPassed: boolean
  smartKnown: boolean
  standby: boolean
  percentUsed: number
}

export interface ArrayMember {
  device: string
  slot: number
  faulty: boolean
  spare: boolean
}

export interface RaidArray {
  name: string
  level: string
  state: string
  active: boolean
  blocks: number
  members: ArrayMember[] | null
  slotsTotal: number
  slotsActive: number
  degraded: boolean
  syncAction: string
  syncPercent: number
  syncFinishIn: string
}

export interface SourceStatus {
  ok: boolean
  lastOk: number
  lastError: string
  errorAt: number
}

export interface AlertRecord {
  id: number
  rule: string
  target: string
  targetName: string
  severity: string
  message: string
  state: string
  firedAt: number
  resolvedAt: number
  notifiedAt: number
  ackedAt: number
}

export interface EventRecord {
  id: number
  ts: number
  containerId: string
  containerName: string
  type: string
  detail: string
}

export interface LogErrorRecord {
  id: number
  ts: number
  containerId: string
  containerName: string
  kind: string
  stream: string
  line: string
}

export interface Overview {
  host: HostState
  portainer: PortainerState
  stacks: StackSummary[]
  containers: Container[]
  disks: Disk[]
  arrays: RaidArray[]
  alerts: AlertRecord[]
  events: EventRecord[]
  sources: Record<string, SourceStatus>
  dbBytes: number
  startedAt: number
  version: string
  timezone: string
  intervals: { stats: number; host: number }
}

export interface SeriesPoint {
  ts: number
  cpu: number
  cpuMax: number
  mem: number
  memMax: number
  memLimit: number
  netRx: number
  netTx: number
  blkRead: number
  blkWrite: number
}

export interface HostPoint {
  ts: number
  cpu: number
  cpuMax: number
  load1: number
  load5: number
  load15: number
  memUsed: number
  memTotal: number
  swapUsed: number
  swapTotal: number
  netRx: number
  netTx: number
  cpuTemp: number
}

export interface MountPoint {
  ts: number
  mount: string
  used: number
  total: number
}

export interface LogLine {
  ts: number
  stream: string
  level: string
  text: string
}

export interface LogHit extends LogLine {
  containerId: string
  containerName: string
}

export interface ImageUpdate {
  image: string
  localDigest: string
  remoteDigest: string
  updateAvailable: boolean
  checkedAt: number
  error: string
  containers: string[]
}

export interface TempPoint {
  ts: number
  temp: number
}

export type Range = '1h' | '6h' | '24h' | '7d' | '30d'
export const RANGES: Range[] = ['1h', '6h', '24h', '7d', '30d']

export type LiveMessage =
  | { type: 'hello'; data: { ts: number } }
  | { type: 'stats'; data: { ts: number; containers: Record<string, Live>; stacks: StackSummary[] } }
  | { type: 'host'; data: HostState }
  | { type: 'containers'; data: { containers: Container[]; stacks: StackSummary[] } }
  | { type: 'event'; data: EventRecord }
  | { type: 'alert'; data: AlertRecord }
  | { type: 'alert_ack'; data: { id: number; ackedAt: number } }
  | { type: 'disks'; data: { disks: Disk[] | null; arrays: RaidArray[] | null } }
  | { type: 'error'; data: LogErrorRecord }
  | { type: 'errors'; data: { count: number; latest: LogErrorRecord } }
