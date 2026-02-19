import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import ConfigPage from './ConfigPage';
import {
  labConfigHandlers,
  configFileHandlers,
  cbtOverridesHandlers,
  configRegenerateHandlers,
} from '@/stories/handlers';

const meta = {
  title: 'Components/ConfigPage',
  component: ConfigPage,
  decorators: [
    Story => (
      <div className="h-[700px] bg-bg">
        <Story />
      </div>
    ),
  ],
  parameters: {
    msw: {
      handlers: [...labConfigHandlers, ...configFileHandlers, ...cbtOverridesHandlers, ...configRegenerateHandlers],
    },
  },
  args: {
    onBack: fn(),
  },
} satisfies Meta<typeof ConfigPage>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};
