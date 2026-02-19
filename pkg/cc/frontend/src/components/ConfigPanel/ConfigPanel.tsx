import type { ConfigInfo, ServiceInfo } from '@/types';
import Spinner from '@/components/Spinner';

interface ConfigPanelProps {
  config: ConfigInfo | null;
  services: ServiceInfo[];
  onNavigateConfig?: () => void;
}

export default function ConfigPanel({ config, services, onNavigateConfig }: ConfigPanelProps) {
  if (!config) {
    return (
      <div className="rounded-xs border border-border bg-surface-light p-4">
        <h3 className="mb-2 text-sm/5 font-semibold text-text-tertiary">Config</h3>
        <Spinner />
      </div>
    );
  }

  const enabledNetworks = config.networks.filter(n => n.enabled);

  // Build a lookup from service name to its data for port/URL resolution.
  const svcMap = new Map(services.map(s => [s.name, s]));

  // Helper to get the primary port for a service, falling back to computed value.
  const getPort = (serviceName: string, fallback: number): number => {
    const svc = svcMap.get(serviceName);
    return svc?.ports?.[0] ?? fallback;
  };

  // Helper to get the URL for a service, falling back to computed value.
  const getUrl = (serviceName: string, fallback: string, suffix = ''): string => {
    const svc = svcMap.get(serviceName);
    const base = svc?.url ?? fallback;
    return base !== '-' ? `${base}${suffix}` : '';
  };

  return (
    <div className="rounded-xs border border-border bg-surface-light p-4">
      <div className="mb-3 flex items-center justify-between">
        <h3 className="text-sm/5 font-semibold text-text-tertiary">Config</h3>
        {onNavigateConfig && (
          <button
            onClick={onNavigateConfig}
            className="text-xs/4 text-accent-light transition-colors hover:text-accent-light/80"
          >
            Manage
          </button>
        )}
      </div>

      <div className="flex flex-col gap-3 text-xs/4">
        {/* Mode + Networks inline */}
        <div className="flex items-center gap-2">
          <span className="rounded-xs bg-accent/20 px-2 py-0.5 font-medium text-accent-light">{config.mode}</span>
          {enabledNetworks.map(n => (
            <span key={n.name} className="rounded-xs bg-success/20 px-2 py-0.5 text-success">
              {n.name}
            </span>
          ))}
        </div>

        {/* Services — per-network ports */}
        <div className="border-t border-border/50 pt-3">
          <div className="mb-2 text-[10px]/3 font-semibold tracking-wider text-text-disabled uppercase">Services</div>
          <div className="flex flex-col gap-0.5">
            <PortRow
              label="Lab Frontend"
              port={getPort('lab-frontend', config.ports.labFrontend)}
              href={getUrl('lab-frontend', `http://localhost:${config.ports.labFrontend}`)}
            />
            {enabledNetworks.map(n => {
              const name = `cbt-api-${n.name}`;
              const fallbackPort = config.ports.cbtApiBase + n.portOffset;
              const port = getPort(name, fallbackPort);
              return (
                <PortRow
                  key={name}
                  label={`CBT API${enabledNetworks.length > 1 ? ` (${n.name})` : ''}`}
                  port={port}
                  href={`http://localhost:${port}/docs`}
                />
              );
            })}
            {enabledNetworks.map(n => {
              const name = `cbt-${n.name}`;
              const fallbackPort = config.ports.cbtFrontendBase + n.portOffset;
              const port = getPort(name, fallbackPort);
              return (
                <PortRow
                  key={`cbt-fe-${n.name}`}
                  label={`CBT Frontend${enabledNetworks.length > 1 ? ` (${n.name})` : ''}`}
                  port={port}
                  href={`http://localhost:${port}`}
                />
              );
            })}
          </div>
        </div>

        {/* Infrastructure */}
        <div className="border-t border-border/50 pt-3">
          <div className="mb-2 text-[10px]/3 font-semibold tracking-wider text-text-disabled uppercase">
            Infrastructure
          </div>
          <div className="flex flex-col gap-0.5">
            <PortRow
              label="ClickHouse CBT"
              port={config.ports.clickhouseCbt}
              href={`http://localhost:${config.ports.clickhouseCbt}/play`}
            />
            {config.mode === 'local' && (
              <PortRow
                label="ClickHouse Xatu"
                port={config.ports.clickhouseXatu}
                href={`http://localhost:${config.ports.clickhouseXatu}/play`}
              />
            )}
            <PortRow label="Redis" port={config.ports.redis} />
          </div>
        </div>

        {/* Observability */}
        <div className="border-t border-border/50 pt-3">
          <div className="mb-2 text-[10px]/3 font-semibold tracking-wider text-text-disabled uppercase">
            Observability
          </div>
          <div className="flex flex-col gap-0.5">
            <PortRow
              label="Prometheus"
              port={config.ports.prometheus}
              href={`http://localhost:${config.ports.prometheus}`}
            />
            <PortRow label="Grafana" port={config.ports.grafana} href={`http://localhost:${config.ports.grafana}`} />
          </div>
        </div>
      </div>
    </div>
  );
}

function PortRow({ label, port, href }: { label: string; port: number; href?: string }) {
  const labelEl = href ? (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="text-text-tertiary transition-colors hover:text-accent-light"
    >
      {label}
    </a>
  ) : (
    <span className="text-text-tertiary">{label}</span>
  );

  return (
    <div className="flex items-center justify-between py-0.5">
      {labelEl}
      <span className="font-mono text-text-secondary">{port}</span>
    </div>
  );
}
