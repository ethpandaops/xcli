import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import { http, HttpResponse, delay } from 'msw';
import CBTOverridesEditor from './CBTOverridesEditor';
import { cbtOverridesHandlers } from '@/stories/handlers';
import { mockCBTOverrides } from '@/stories/fixtures';

const meta = {
  title: 'Components/CBTOverridesEditor',
  component: CBTOverridesEditor,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
  parameters: {
    msw: { handlers: cbtOverridesHandlers },
  },
  args: {
    onToast: fn(),
    stack: 'lab',
  },
} satisfies Meta<typeof CBTOverridesEditor>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const WithMissingDeps: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/stacks/:stack/config/overrides', async () => {
          await delay(150);
          return HttpResponse.json({
            ...mockCBTOverrides,
            transformationModels: mockCBTOverrides.transformationModels.map(m => ({
              ...m,
              enabled: true,
            })),
            externalModels: mockCBTOverrides.externalModels.map(m => ({
              ...m,
              enabled: m.name === 'beacon_api_eth_v1_beacon_block',
            })),
          });
        }),
        ...cbtOverridesHandlers.slice(1),
      ],
    },
  },
};

export const Loading: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/stacks/:stack/config/overrides', async () => {
          await delay(999999);
          return HttpResponse.json(mockCBTOverrides);
        }),
        http.get('/api/stacks/:stack/config', async () => {
          await delay(999999);
          return HttpResponse.json({ mode: 'local' });
        }),
      ],
    },
  },
};
