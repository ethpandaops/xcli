import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
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
  args: {
    onSelectTab: fn(),
    onCloseTab: fn(),
  },
} satisfies Meta<typeof LogViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AllServices: Story = {
  args: {
    logs: mockLogs,
    activeTab: null,
    openTabs: [],
  },
};

export const WithServiceTab: Story = {
  args: {
    logs: mockLogs,
    activeTab: 'lab-backend',
    openTabs: ['lab-backend'],
  },
};

export const MultipleTabs: Story = {
  args: {
    logs: mockLogs,
    activeTab: 'lab-backend',
    openTabs: ['lab-backend', 'cbt-mainnet', 'xatu-cbt-mainnet'],
  },
};

export const Empty: Story = {
  args: {
    logs: [],
    activeTab: null,
    openTabs: [],
  },
};
