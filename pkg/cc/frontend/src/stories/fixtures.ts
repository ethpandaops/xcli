import type {
  ServiceInfo,
  InfraInfo,
  RepoInfo,
  LogLine,
  ConfigInfo,
  LabConfigFull,
  ConfigFileInfo,
  ConfigFileContent,
  CBTOverridesState,
  StackStatus,
  GitResponse,
  StatusResponse,
} from '@/types';

// --- Services ---

export const mockServices: ServiceInfo[] = [
  {
    name: 'lab-backend',
    status: 'running',
    pid: 12345,
    uptime: '2h 15m',
    url: 'http://localhost:19280',
    ports: [19280],
    health: 'healthy',
    logFile: '/tmp/lab-backend.log',
  },
  {
    name: 'cbt-api-mainnet',
    status: 'running',
    pid: 12346,
    uptime: '2h 14m',
    url: 'http://localhost:19300',
    ports: [19300],
    health: 'unhealthy',
    logFile: '/tmp/cbt-api-mainnet.log',
  },
  {
    name: 'cbt-frontend-mainnet',
    status: 'stopped',
    pid: 0,
    uptime: '',
    url: '',
    ports: [19400],
    health: 'unknown',
    logFile: '/tmp/cbt-frontend-mainnet.log',
  },
  {
    name: 'xatu-cbt-mainnet',
    status: 'crashed',
    pid: 0,
    uptime: '',
    url: '',
    ports: [],
    health: 'unknown',
    logFile: '/tmp/xatu-cbt-mainnet.log',
  },
  {
    name: 'prometheus',
    status: 'running',
    pid: 12350,
    uptime: '2h 15m',
    url: 'http://localhost:9090',
    ports: [9090],
    health: 'healthy',
    logFile: '',
  },
];

// --- Infrastructure ---

export const mockInfrastructure: InfraInfo[] = [
  { name: 'clickhouse-cbt', status: 'running', type: 'clickhouse' },
  { name: 'redis', status: 'running', type: 'redis' },
];

// --- Repos ---

export const mockRepos: RepoInfo[] = [
  {
    name: 'lab',
    path: '/home/user/repos/lab',
    branch: 'main',
    aheadBy: 0,
    behindBy: 0,
    hasUncommitted: false,
    uncommittedCount: 0,
    latestTag: 'v1.2.0',
    commitsSinceTag: 0,
    isUpToDate: true,
  },
  {
    name: 'cbt',
    path: '/home/user/repos/cbt',
    branch: 'main',
    aheadBy: 0,
    behindBy: 3,
    hasUncommitted: false,
    uncommittedCount: 0,
    latestTag: 'v0.8.1',
    commitsSinceTag: 5,
    isUpToDate: false,
  },
  {
    name: 'xatu-cbt',
    path: '/home/user/repos/xatu-cbt',
    branch: 'feature/new-model',
    aheadBy: 2,
    behindBy: 0,
    hasUncommitted: true,
    uncommittedCount: 4,
    latestTag: 'v0.3.0',
    commitsSinceTag: 12,
    isUpToDate: false,
  },
];

// --- Logs ---

const now = new Date();
function ts(offsetMs: number): string {
  return new Date(now.getTime() + offsetMs).toISOString();
}

export const mockLogs: LogLine[] = [
  { Service: 'lab-backend', Timestamp: ts(-20000), Level: 'INFO', Message: 'Server started on :19280', Raw: '' },
  { Service: 'lab-backend', Timestamp: ts(-19000), Level: 'INFO', Message: 'Connected to ClickHouse', Raw: '' },
  { Service: 'lab-backend', Timestamp: ts(-18000), Level: 'DEBUG', Message: 'Loading network configs', Raw: '' },
  {
    Service: 'cbt-api-mainnet',
    Timestamp: ts(-17000),
    Level: 'INFO',
    Message: 'API server listening on :19300',
    Raw: '',
  },
  { Service: 'cbt-api-mainnet', Timestamp: ts(-16000), Level: 'WARN', Message: 'Slow query detected: 2.3s', Raw: '' },
  { Service: 'lab-backend', Timestamp: ts(-15000), Level: 'INFO', Message: 'Health check passed', Raw: '' },
  { Service: 'xatu-cbt-mainnet', Timestamp: ts(-14000), Level: 'INFO', Message: 'Starting block processor', Raw: '' },
  {
    Service: 'xatu-cbt-mainnet',
    Timestamp: ts(-13000),
    Level: 'ERROR',
    Message: 'Connection refused: clickhouse:9000',
    Raw: '',
  },
  { Service: 'xatu-cbt-mainnet', Timestamp: ts(-12000), Level: 'ERROR', Message: 'Retry attempt 1/3 failed', Raw: '' },
  {
    Service: 'lab-backend',
    Timestamp: ts(-11000),
    Level: 'DEBUG',
    Message: 'Cache invalidated for network mainnet',
    Raw: '',
  },
  {
    Service: 'cbt-api-mainnet',
    Timestamp: ts(-10000),
    Level: 'INFO',
    Message: 'Processing batch of 150 blocks',
    Raw: '',
  },
  { Service: 'cbt-api-mainnet', Timestamp: ts(-9000), Level: 'INFO', Message: 'Batch complete in 450ms', Raw: '' },
  {
    Service: 'lab-backend',
    Timestamp: ts(-8000),
    Level: 'INFO',
    Message: 'SSE client connected: 192.168.1.10',
    Raw: '',
  },
  { Service: 'xatu-cbt-mainnet', Timestamp: ts(-7000), Level: 'WARN', Message: 'High memory usage: 85%', Raw: '' },
  {
    Service: 'cbt-api-mainnet',
    Timestamp: ts(-6000),
    Level: 'DEBUG',
    Message: 'Query plan optimized for slot range',
    Raw: '',
  },
  { Service: 'lab-backend', Timestamp: ts(-5000), Level: 'INFO', Message: 'Config reloaded successfully', Raw: '' },
  {
    Service: 'xatu-cbt-mainnet',
    Timestamp: ts(-4000),
    Level: 'ERROR',
    Message: 'Panic recovered: index out of range',
    Raw: '',
  },
  { Service: 'cbt-api-mainnet', Timestamp: ts(-3000), Level: 'INFO', Message: 'Serving /docs on :19300', Raw: '' },
  { Service: 'lab-backend', Timestamp: ts(-2000), Level: 'INFO', Message: 'Metrics exported to Prometheus', Raw: '' },
  { Service: 'prometheus', Timestamp: ts(-1000), Level: 'INFO', Message: 'Scrape complete: 23 targets', Raw: '' },
];

// --- Config ---

export const mockConfig: ConfigInfo = {
  mode: 'local',
  networks: [
    { name: 'mainnet', enabled: true, portOffset: 0 },
    { name: 'holesky', enabled: true, portOffset: 100 },
  ],
  ports: {
    labBackend: 19280,
    labFrontend: 19281,
    cbtBase: 19300,
    cbtApiBase: 19300,
    cbtFrontendBase: 19400,
    clickhouseCbt: 9001,
    clickhouseXatu: 9002,
    redis: 6379,
    prometheus: 9090,
    grafana: 3000,
  },
  cfgPath: '/home/user/.xcli/config.yaml',
};

// --- Lab Config Full ---

export const mockLabConfig: LabConfigFull = {
  mode: 'local',
  networks: [
    { Name: 'mainnet', Enabled: true, PortOffset: 0 },
    { Name: 'holesky', Enabled: true, PortOffset: 100 },
  ],
  infrastructure: {
    ClickHouse: {
      Xatu: { Mode: 'local' },
      CBT: { Mode: 'local' },
    },
    Redis: { Port: 6379 },
    Volumes: { Persist: true },
    Observability: { Enabled: true, PrometheusPort: 9090, GrafanaPort: 3000 },
    ClickHouseXatuPort: 9002,
    ClickHouseCBTPort: 9001,
    RedisPort: 6379,
  },
  ports: {
    LabBackend: 19280,
    LabFrontend: 19281,
    CBTBase: 19300,
    CBTAPIBase: 19300,
    CBTFrontendBase: 19400,
  },
  dev: {
    LabRebuildOnChange: true,
    HotReload: false,
  },
  repos: {
    CBT: '/home/user/repos/cbt',
    XatuCBT: '/home/user/repos/xatu-cbt',
    CBTAPI: '/home/user/repos/cbt-api',
    LabBackend: '/home/user/repos/lab-backend',
    Lab: '/home/user/repos/lab',
  },
};

// --- Config Files ---

export const mockConfigFiles: ConfigFileInfo[] = [
  { name: 'cbt-api-mainnet.yaml', hasOverride: false, size: 1240 },
  { name: 'xatu-cbt-mainnet.yaml', hasOverride: true, size: 890 },
  { name: 'docker-compose.yaml', hasOverride: false, size: 3200 },
];

export const mockConfigFileContent: ConfigFileContent = {
  name: 'xatu-cbt-mainnet.yaml',
  content: `clickhouse:
  host: localhost
  port: 9001
  database: cbt_mainnet

processing:
  batch_size: 100
  workers: 4
  retry_attempts: 3

logging:
  level: info
  format: json
`,
  hasOverride: true,
  overrideContent: `clickhouse:
  host: localhost
  port: 9001
  database: cbt_mainnet

processing:
  batch_size: 200
  workers: 8
  retry_attempts: 5

logging:
  level: debug
  format: json
`,
};

// --- CBT Overrides ---

export const mockCBTOverrides: CBTOverridesState = {
  externalModels: [
    { name: 'beacon_api_eth_v1_beacon_block', overrideKey: 'beacon_api_eth_v1_beacon_block', enabled: true },
    {
      name: 'beacon_api_eth_v1_events_attestation',
      overrideKey: 'beacon_api_eth_v1_events_attestation',
      enabled: true,
    },
    { name: 'beacon_api_eth_v1_events_head', overrideKey: 'beacon_api_eth_v1_events_head', enabled: false },
    { name: 'beacon_api_eth_v2_beacon_block', overrideKey: 'beacon_api_eth_v2_beacon_block', enabled: true },
    { name: 'mempool_transaction', overrideKey: 'mempool_transaction', enabled: false },
  ],
  transformationModels: [
    { name: 'block_timing', overrideKey: 'block_timing', enabled: true },
    { name: 'attestation_coverage', overrideKey: 'attestation_coverage', enabled: true },
    { name: 'slot_analysis', overrideKey: 'slot_analysis', enabled: false },
    { name: 'proposer_timing', overrideKey: 'proposer_timing', enabled: false },
  ],
  dependencies: {
    block_timing: ['beacon_api_eth_v1_beacon_block', 'beacon_api_eth_v2_beacon_block'],
    attestation_coverage: ['beacon_api_eth_v1_events_attestation'],
    slot_analysis: ['beacon_api_eth_v1_events_head', 'beacon_api_eth_v1_beacon_block'],
    proposer_timing: ['beacon_api_eth_v1_beacon_block'],
  },
  envMinTimestamp: '',
  envTimestampEnabled: false,
  envMinBlock: '',
  envBlockEnabled: false,
};

// --- Stack Status ---

export const mockStackStatus: StackStatus = {
  status: 'running',
  runningServices: 3,
  totalServices: 5,
};

// --- Composite Responses ---

export const mockGitResponse: GitResponse = {
  repos: mockRepos,
};

export const mockStatusResponse: StatusResponse = {
  services: mockServices,
  infrastructure: mockInfrastructure,
  config: mockConfig,
  timestamp: new Date().toISOString(),
};
