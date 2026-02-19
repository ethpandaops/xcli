import type { Meta, StoryObj } from '@storybook/react-vite';
import GitStatus from './GitStatus';
import { mockRepos } from '@/stories/fixtures';

const meta = {
  title: 'Components/GitStatus',
  component: GitStatus,
  decorators: [
    Story => (
      <div className="bg-bg p-8">
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof GitStatus>;

export default meta;
type Story = StoryObj<typeof meta>;

export const UpToDate: Story = {
  args: {
    repos: [mockRepos[0]],
  },
};

export const WithDrift: Story = {
  args: {
    repos: mockRepos,
  },
};

export const WithUncommitted: Story = {
  args: {
    repos: [mockRepos[2]],
  },
};

export const WithError: Story = {
  args: {
    repos: [
      {
        name: 'broken-repo',
        path: '/tmp/broken',
        branch: 'main',
        aheadBy: 0,
        behindBy: 0,
        hasUncommitted: false,
        uncommittedCount: 0,
        latestTag: '',
        commitsSinceTag: 0,
        isUpToDate: false,
        error: 'fatal: not a git repository',
      },
    ],
  },
};

export const Loading: Story = {
  args: { repos: [] },
};
