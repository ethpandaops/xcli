import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import ServiceCard from './ServiceCard';
import { mockServices } from '@/stories/fixtures';
import { serviceActionHandlers } from '@/stories/handlers';

const meta = {
  title: 'Components/ServiceCard',
  component: ServiceCard,
  decorators: [
    Story => (
      <div className="w-80 bg-bg p-8">
        <Story />
      </div>
    ),
  ],
  parameters: {
    msw: { handlers: serviceActionHandlers },
  },
  args: {
    selected: false,
    onSelect: fn(),
  },
} satisfies Meta<typeof ServiceCard>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Running: Story = {
  args: { service: mockServices[0] },
};

export const Stopped: Story = {
  args: { service: mockServices[2] },
};

export const Crashed: Story = {
  args: { service: mockServices[3] },
};

export const Unhealthy: Story = {
  args: { service: mockServices[1] },
};

export const Selected: Story = {
  args: {
    service: mockServices[0],
    selected: true,
  },
};

export const DockerService: Story = {
  args: { service: mockServices[4] },
};
