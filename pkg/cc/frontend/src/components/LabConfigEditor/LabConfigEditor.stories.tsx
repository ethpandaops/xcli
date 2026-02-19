import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import { http, HttpResponse, delay } from 'msw';
import LabConfigEditor from './LabConfigEditor';
import { labConfigHandlers } from '@/stories/handlers';
import { mockLabConfig } from '@/stories/fixtures';

const meta = {
  title: 'Components/LabConfigEditor',
  component: LabConfigEditor,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
  parameters: {
    msw: { handlers: labConfigHandlers },
  },
  args: {
    onToast: fn(),
  },
} satisfies Meta<typeof LabConfigEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const HybridMode: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/config/lab', async () => {
          await delay(150);
          return HttpResponse.json({
            ...mockLabConfig,
            mode: 'hybrid',
            infrastructure: {
              ...mockLabConfig.infrastructure,
              ClickHouse: {
                Xatu: { Mode: 'external', ExternalURL: 'clickhouse.example.com:9000' },
                CBT: { Mode: 'local' },
              },
            },
          });
        }),
        ...labConfigHandlers.slice(1),
      ],
    },
  },
};

export const Loading: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/config/lab', async () => {
          await delay(999999);
          return HttpResponse.json(mockLabConfig);
        }),
      ],
    },
  },
};

export const ErrorState: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/config/lab', async () => {
          await delay(100);
          return new HttpResponse(null, { status: 500, statusText: 'Internal Server Error' });
        }),
      ],
    },
  },
};

export const WithNavigateBack: Story = {
  args: {
    onNavigateDashboard: fn(),
  },
};
