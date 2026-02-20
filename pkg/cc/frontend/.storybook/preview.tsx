import type { Preview, ReactRenderer } from '@storybook/react-vite';
import { initialize, mswLoader } from 'msw-storybook-addon';
import { withThemeByClassName } from '@storybook/addon-themes';
import { ThemeProvider } from '../src/hooks/useTheme';
import '../src/index.css';

// Initialize MSW
initialize({
  onUnhandledRequest: 'bypass',
  quiet: true,
});

const preview: Preview = {
  loaders: [mswLoader],
  decorators: [
    Story => (
      <ThemeProvider>
        <Story />
      </ThemeProvider>
    ),
    withThemeByClassName<ReactRenderer>({
      themes: {
        light: '',
        dark: 'dark',
      },
      defaultTheme: 'dark',
    }),
  ],
  parameters: {
    msw: {
      handlers: [],
    },
    controls: {
      matchers: {
        color: /(background|color)$/i,
        date: /Date$/i,
      },
    },
    options: {
      storySort: {
        method: 'alphabetical',
      },
    },
  },
};

export default preview;
