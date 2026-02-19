import type { Meta, StoryObj } from '@storybook/react-vite';
import InfraPanel from './InfraPanel';
import { mockInfrastructure } from '@/stories/fixtures';

const meta = {
  title: 'Components/InfraPanel',
  component: InfraPanel,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof InfraPanel>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllRunning: Story = {
  args: { infrastructure: mockInfrastructure },
};

export const Mixed: Story = {
  args: {
    infrastructure: [
      { name: 'clickhouse-cbt', status: 'running', type: 'clickhouse' },
      { name: 'redis', status: 'stopped', type: 'redis' },
    ],
  },
};

export const Empty: Story = {
  args: { infrastructure: [] },
};
