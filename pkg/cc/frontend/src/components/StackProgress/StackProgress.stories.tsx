import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import StackProgress, { BOOT_PHASES, STOP_PHASES, derivePhaseStates } from './StackProgress';

const meta = {
  title: 'Components/StackProgress',
  component: StackProgress,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof StackProgress>;

export default meta;
type Story = StoryObj<typeof meta>;

export const BootInProgress: Story = {
  args: {
    phases: derivePhaseStates(
      [
        { phase: 'prerequisites', message: 'Done' },
        { phase: 'build_xatu_cbt', message: 'Done' },
        { phase: 'infrastructure', message: 'Starting ClickHouse...' },
      ],
      null
    ),
    error: null,
    title: 'Booting Stack',
  },
};

export const BootComplete: Story = {
  args: {
    phases: derivePhaseStates(
      [
        { phase: 'prerequisites', message: '' },
        { phase: 'build_xatu_cbt', message: '' },
        { phase: 'infrastructure', message: '' },
        { phase: 'build_services', message: '' },
        { phase: 'network_setup', message: '' },
        { phase: 'generate_configs', message: '' },
        { phase: 'build_cbt_api', message: '' },
        { phase: 'start_services', message: '' },
        { phase: 'complete', message: '' },
      ],
      null
    ),
    error: null,
    title: 'Booting Stack',
  },
};

export const BootError: Story = {
  args: {
    phases: derivePhaseStates(
      [
        { phase: 'prerequisites', message: 'Done' },
        { phase: 'build_xatu_cbt', message: 'Building...' },
      ],
      'Build failed: missing dependency github.com/example/pkg'
    ),
    error: 'Build failed: missing dependency github.com/example/pkg',
    title: 'Booting Stack',
    onRetry: fn(),
  },
};

export const StopInProgress: Story = {
  args: {
    phases: derivePhaseStates(
      [
        { phase: 'stop_services', message: 'Done' },
        { phase: 'cleanup_orphans', message: 'Cleaning...' },
      ],
      null,
      STOP_PHASES
    ),
    error: null,
    title: 'Stopping Stack',
  },
};

export const WithCancel: Story = {
  args: {
    phases: derivePhaseStates([{ phase: 'prerequisites', message: 'Checking...' }], null, BOOT_PHASES),
    error: null,
    title: 'Booting Stack',
    onCancel: fn(),
  },
};
