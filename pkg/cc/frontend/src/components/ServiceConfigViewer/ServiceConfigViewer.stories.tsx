import type { Meta, StoryObj } from '@storybook/react-vite';
import { fn } from 'storybook/test';
import { http, HttpResponse, delay } from 'msw';
import ServiceConfigViewer from './ServiceConfigViewer';
import { configFileHandlers } from '@/stories/handlers';

const meta = {
  title: 'Components/ServiceConfigViewer',
  component: ServiceConfigViewer,
  decorators: [
    Story => (
      <div className="h-[600px] bg-bg p-8">
        <Story />
      </div>
    ),
  ],
  parameters: {
    msw: { handlers: configFileHandlers },
  },
  args: {
    onToast: fn(),
  },
} satisfies Meta<typeof ServiceConfigViewer>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

export const WithOverride: Story = {};

export const EmptyFileList: Story = {
  parameters: {
    msw: {
      handlers: [
        http.get('/api/config/files', async () => {
          await delay(100);
          return HttpResponse.json([]);
        }),
      ],
    },
  },
};
