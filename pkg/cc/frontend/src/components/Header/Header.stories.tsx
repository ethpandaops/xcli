import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import Header from './Header';
import { mockServices, mockInfrastructure } from '@/stories/fixtures';

const meta = {
  title: 'Components/Header',
  component: Header,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
  args: {
    services: mockServices,
    infrastructure: mockInfrastructure,
    mode: 'local',
    stackStatus: 'stopped',
    onStackAction: fn(),
    notificationsEnabled: true,
    onToggleNotifications: fn(),
  },
} satisfies Meta<typeof Header>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Stopped: Story = {};

export const Running: Story = {
  args: { stackStatus: 'running' },
};

export const Starting: Story = {
  args: {
    stackStatus: 'starting',
    currentPhase: 'Build Services',
  },
};

export const Stopping: Story = {
  args: {
    stackStatus: 'stopping',
    currentPhase: 'Stop Infrastructure',
  },
};

export const WithConfigButton: Story = {
  args: {
    stackStatus: 'running',
    onNavigateConfig: fn(),
  },
};
