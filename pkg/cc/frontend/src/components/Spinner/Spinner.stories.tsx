import type { Meta, StoryObj } from '@storybook/react-vite';
import Spinner from './Spinner';

const meta = {
  title: 'Components/Spinner',
  component: Spinner,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof Spinner>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const Centered: Story = {
  args: { centered: true },
  decorators: [
    Story => (
      <div className="h-64">
        <Story />
      </div>
    ),
  ],
};

export const WithCustomText: Story = {
  args: { text: 'Loading config' },
};

export const MediumSize: Story = {
  args: { size: 'md' },
};
