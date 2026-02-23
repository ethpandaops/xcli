export interface ServiceInfo {
  name: string;
  status: string;
  pid: number;
  uptime: string;
  url: string;
  ports: number[];
  health: string;
  logFile: string;
}

export interface InfraInfo {
  name: string;
  status: string;
  type: string;
}

export interface HealthStatus {
  [service: string]: {
    Status: string;
    LastCheck: string;
    LastError: string;
    ConsecutiveFailures: number;
  };
}

export interface LogLine {
  Service: string;
  Timestamp: string;
  Level: string;
  Message: string;
  Raw: string;
}

export interface NetworkInfo {
  name: string;
  enabled: boolean;
  portOffset: number;
}

export interface ConfigInfo {
  mode: string;
  networks: NetworkInfo[];
  ports: {
    labBackend: number;
    labFrontend: number;
    cbtBase: number;
    cbtApiBase: number;
    cbtFrontendBase: number;
    clickhouseCbt: number;
    clickhouseXatu: number;
    redis: number;
    prometheus: number;
    grafana: number;
  };
  cfgPath: string;
}

export interface StatusResponse {
  services: ServiceInfo[];
  infrastructure: InfraInfo[];
  config: ConfigInfo;
  timestamp: string;
}

export interface RepoInfo {
  name: string;
  path: string;
  branch: string;
  aheadBy: number;
  behindBy: number;
  hasUncommitted: boolean;
  uncommittedCount: number;
  latestTag: string;
  commitsSinceTag: number;
  isUpToDate: boolean;
  error?: string;
}

export interface GitResponse {
  repos: RepoInfo[];
}

export interface StackStatus {
  status: string;
  runningServices: number;
  totalServices: number;
  error?: string;
  progress?: StackProgressEvent[];
}

export interface StackProgressEvent {
  phase: string;
  message: string;
}

export interface ProviderCapabilities {
  streaming: boolean;
  interrupt: boolean;
  sessions: boolean;
}

export interface AIProviderInfo {
  id: string;
  label: string;
  default: boolean;
  available: boolean;
  capabilities: ProviderCapabilities;
}

export type RedisValueMode = 'text' | 'base64';

export interface RedisEncodedValue {
  mode: RedisValueMode;
  text?: string;
  base64?: string;
}

export interface RedisHashEntry {
  field: string;
  value: RedisEncodedValue;
}

export interface RedisZSetEntry {
  member: RedisEncodedValue;
  score: number;
}

export interface RedisKeyMeta {
  totalItems?: number;
  sizeBytes?: number;
  truncated?: boolean;
}

export interface RedisStatusResponse {
  connected: boolean;
  addr: string;
  db: number;
  ping: string;
  dbSize: number;
}

export interface RedisTreeResponse {
  db: number;
  prefix: string;
  count: number;
  cursor: number;
  nextCursor: number;
  scanned: number;
  truncated: boolean;
  branches: string[];
  leaves: string[];
}

export interface RedisSearchResponse {
  db: number;
  query: string;
  count: number;
  cursor: number;
  nextCursor: number;
  scanned: number;
  truncated: boolean;
  keys: string[];
}

export interface RedisKeyResponse {
  db: number;
  key: string;
  type: 'string' | 'hash' | 'list' | 'set' | 'zset' | string;
  ttlMs: number;
  version: string;
  meta: RedisKeyMeta;
  stringValue?: RedisEncodedValue;
  hashEntries?: RedisHashEntry[];
  listItems?: RedisEncodedValue[];
  setMembers?: RedisEncodedValue[];
  zsetMembers?: RedisZSetEntry[];
}

export interface RedisWriteRequest {
  db: number;
  key: string;
  type: 'string' | 'hash' | 'list' | 'set' | 'zset';
  expectedVersion?: string;
  ttlMode?: 'none' | 'keep' | 'set' | 'clear';
  ttlSeconds?: number;
  stringValue?: RedisEncodedValue;
  hashEntries?: RedisHashEntry[];
  listItems?: RedisEncodedValue[];
  setMembers?: RedisEncodedValue[];
  zsetMembers?: RedisZSetEntry[];
}

export interface RedisDeleteManyRequest {
  db: number;
  keys: string[];
}

export interface RedisDeleteManyResult {
  key: string;
  deleted: boolean;
  error?: string;
}

export interface RedisDeleteManyResponse {
  db: number;
  results: RedisDeleteManyResult[];
}

// Config management types
// Note: nested config types use Go's PascalCase field names since
// the Go structs only have yaml tags, not json tags.

export interface ClickHouseClusterConfig {
  Mode: string;
  ExternalURL?: string;
  ExternalDatabase?: string;
  ExternalUsername?: string;
  ExternalPassword?: string;
}

export interface ClickHouseConfig {
  Xatu: ClickHouseClusterConfig;
  CBT: ClickHouseClusterConfig;
}

export interface ObservabilityConfig {
  Enabled: boolean;
  PrometheusPort?: number;
  GrafanaPort?: number;
}

export interface InfrastructureConfig {
  ClickHouse: ClickHouseConfig;
  Redis: { Port: number };
  Volumes: { Persist: boolean };
  Observability: ObservabilityConfig;
  ClickHouseXatuPort: number;
  ClickHouseCBTPort: number;
  RedisPort: number;
}

export interface LabPortsConfig {
  LabBackend: number;
  LabFrontend: number;
  CBTBase: number;
  CBTAPIBase: number;
  CBTFrontendBase: number;
}

export interface LabDevConfig {
  LabRebuildOnChange: boolean;
  HotReload: boolean;
  XatuRef?: string;
}

export interface LabReposConfig {
  CBT: string;
  XatuCBT: string;
  CBTAPI: string;
  LabBackend: string;
  Lab: string;
}

export interface LabNetworkConfig {
  Name: string;
  Enabled: boolean;
  PortOffset: number;
  GenesisTimestamp?: number;
}

export interface LabConfigFull {
  mode: string;
  networks: LabNetworkConfig[];
  infrastructure: InfrastructureConfig;
  ports: LabPortsConfig;
  dev: LabDevConfig;
  repos: LabReposConfig;
}

export interface ConfigFileInfo {
  name: string;
  hasOverride: boolean;
  size: number;
}

export interface ConfigFileContent {
  name: string;
  content: string;
  hasOverride: boolean;
  overrideContent?: string;
}

export interface ModelEntry {
  name: string;
  overrideKey: string;
  enabled: boolean;
}

export interface StackInfo {
  name: string;
  label: string;
  status?: string;
}

export interface DiagnosisReport {
  rootCause: string;
  explanation: string;
  affectedFiles: string[];
  suggestions: string[];
  fixCommands: string[];
}

export type AIDiagnosis = DiagnosisReport;

export interface DiagnosisTurn {
  prompt?: string;
  thinking: string;
  activity: string;
  answer: string;
}

export interface CBTOverridesState {
  defaultEnabled?: boolean;
  externalModels: ModelEntry[];
  transformationModels: ModelEntry[];
  dependencies: Record<string, string[]>;
  envMinTimestamp: string;
  envTimestampEnabled: boolean;
  envMinBlock: string;
  envBlockEnabled: boolean;
}
