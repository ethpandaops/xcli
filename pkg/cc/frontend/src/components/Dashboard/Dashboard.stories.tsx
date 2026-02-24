import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import { http, HttpResponse, delay } from 'msw';
import Dashboard from './Dashboard';
import { allHandlers } from '@/stories/handlers';
import { mockStatusResponse, mockStackStatus, mockServices } from '@/stories/fixtures';

const meta = {
  title: 'Components/Dashboard',
  component: Dashboard,
  parameters: {
    msw: { handlers: allHandlers },
  },
  args: {
    onNavigateConfig: fn(),
    stack: 'lab',
    availableStacks: ['lab'],
    onSwitchStack: fn(),
    capabilities: {
      hasEditableConfig: true,
      hasServiceConfigs: true,
      hasCbtOverrides: true,
      hasRedis: true,
      hasGitRepos: true,
      hasRegenerate: true,
      hasRebuild: true,
    },
  },
} satisfies Meta<typeof Dashboard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {};

export const Stopped: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/stacks/:stack/status', async () => {
          await delay(100);
          return HttpResponse.json({
            ...mockStatusResponse,
            services: mockServices.map(s => ({ ...s, status: 'stopped', pid: 0, uptime: '' })),
          });
        }),
        http.get('/api/stacks/:stack/stack/status', async () => {
          await delay(100);
          return HttpResponse.json({
            ...mockStackStatus,
            status: 'stopped',
            runningServices: 0,
          });
        }),
        ...allHandlers.slice(2),
      ],
    },
  },
};

export const Starting: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/stacks/:stack/status', async () => {
          await delay(100);
          return HttpResponse.json({
            ...mockStatusResponse,
            services: [],
          });
        }),
        http.get('/api/stacks/:stack/stack/status', async () => {
          await delay(100);
          return HttpResponse.json({
            status: 'starting',
            runningServices: 0,
            totalServices: 5,
            progress: [
              { phase: 'prerequisites', message: 'Done' },
              { phase: 'build_xatu_cbt', message: 'Building...' },
            ],
          });
        }),
        ...allHandlers.slice(2),
      ],
    },
  },
};
