import type { Meta, StoryObj } from '@storybook/react-vite';
import ConfigPanel from './ConfigPanel';
import { mockConfig, mockServices } from '@/stories/fixtures';

const meta = {
  title: 'Components/ConfigPanel',
  component: ConfigPanel,
  decorators: [
    Story => (
      <div className="w-72 bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof ConfigPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: {
    config: mockConfig,
    services: mockServices,
  },
};

export const Loading: Story = {
  args: {
    config: null,
    services: [],
  },
};

export const MultipleNetworks: Story = {
  args: {
    config: {
      ...mockConfig,
      networks: [
        { name: 'mainnet', enabled: true, portOffset: 0 },
        { name: 'holesky', enabled: true, portOffset: 100 },
        { name: 'sepolia', enabled: true, portOffset: 200 },
      ],
    },
    services: mockServices,
  },
};
