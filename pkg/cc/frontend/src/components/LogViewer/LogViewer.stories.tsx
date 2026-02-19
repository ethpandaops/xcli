import type { Meta, StoryObj } from '@storybook/react-vite';
import LogViewer from './LogViewer';
import { mockLogs } from '@/stories/fixtures';

const meta = {
  title: 'Components/LogViewer',
  component: LogViewer,
  decorators: [
    Story => (
      <div className="h-[500px] bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof LogViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllServices: Story = {
  args: {
    logs: mockLogs,
    selectedService: null,
  },
};

export const FilteredToService: Story = {
  args: {
    logs: mockLogs,
    selectedService: 'lab-backend',
  },
};

export const MixedLevels: Story = {
  args: {
    logs: mockLogs,
    selectedService: 'xatu-cbt-mainnet',
  },
};

export const Empty: Story = {
  args: {
    logs: [],
    selectedService: null,
  },
};
