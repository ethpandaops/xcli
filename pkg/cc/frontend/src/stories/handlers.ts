import { http, HttpResponse, delay } from 'msw';
import {
  mockStatusResponse,
  mockStackStatus,
  mockGitResponse,
  mockLogs,
  mockLabConfig,
  mockConfigFiles,
  mockConfigFileContent,
  mockCBTOverrides,
  mockConfig,
} from './fixtures';

// --- Status ---

export const statusHandlers = [
  http.get('/api/stacks/:stack/status', async () => {
    await delay(100);
    return HttpResponse.json(mockStatusResponse);
  }),
  http.get('/api/stacks/:stack/stack/status', async () => {
    await delay(100);
    return HttpResponse.json(mockStackStatus);
  }),
];

// --- Git ---

export const gitHandlers = [
  http.get('/api/stacks/:stack/git', async () => {
    await delay(100);
    return HttpResponse.json(mockGitResponse);
  }),
];

// --- Logs ---

export const logHandlers = [
  http.get('/api/stacks/:stack/logs', async () => {
    await delay(100);
    return HttpResponse.json(mockLogs);
  }),
  http.get('/api/stacks/:stack/services/:name/logs', async () => {
    await delay(100);
    return HttpResponse.json(mockLogs.slice(0, 5));
  }),
];

// --- Stack Actions ---

export const stackActionHandlers = [
  http.post('/api/stacks/:stack/stack/up', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stacks/:stack/stack/down', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stacks/:stack/stack/cancel', async () => {
    await delay(100);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stacks/:stack/stack/restart', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
];

// --- Service Actions ---

export const serviceActionHandlers = [
  http.post('/api/stacks/:stack/services/:name/:action', async () => {
    await delay(500);
    return HttpResponse.json({ status: 'ok' });
  }),
];

// --- Lab Config ---

export const labConfigHandlers = [
  http.get('/api/stacks/:stack/config', async () => {
    await delay(150);
    return HttpResponse.json(mockLabConfig);
  }),
  http.put('/api/stacks/:stack/config', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  ...statusHandlers,
  ...stackActionHandlers,
];

// --- Config Files ---

export const configFileHandlers = [
  http.get('/api/stacks/:stack/config/files', async () => {
    await delay(100);
    return HttpResponse.json(mockConfigFiles);
  }),
  http.get('/api/stacks/:stack/config/files/:name', async () => {
    await delay(150);
    return HttpResponse.json(mockConfigFileContent);
  }),
  http.put('/api/stacks/:stack/config/files/:name/override', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  http.delete('/api/stacks/:stack/config/files/:name/override', async () => {
    await delay(200);
    return HttpResponse.json({ status: 'deleted' });
  }),
];

// --- CBT Overrides ---

export const cbtOverridesHandlers = [
  http.get('/api/stacks/:stack/config/overrides', async () => {
    await delay(150);
    return HttpResponse.json(mockCBTOverrides);
  }),
  http.get('/api/stacks/:stack/config', async () => {
    await delay(100);
    return HttpResponse.json({ mode: mockConfig.mode });
  }),
  http.put('/api/stacks/:stack/config/overrides', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  http.get('/api/stacks/:stack/services', async () => {
    await delay(100);
    return HttpResponse.json(mockStatusResponse.services);
  }),
  ...serviceActionHandlers,
  ...statusHandlers,
];

// --- Config Regenerate ---

export const configRegenerateHandlers = [
  http.post('/api/stacks/:stack/config/regenerate', async () => {
    await delay(500);
    return HttpResponse.json({ status: 'ok' });
  }),
];

// --- Stacks List ---

export const stacksHandlers = [
  http.get('/api/stacks', async () => {
    await delay(50);
    return HttpResponse.json([{ name: 'lab', label: 'Lab' }]);
  }),
];

// --- AI Providers & Diagnose ---

export const aiHandlers = [
  http.get('/api/stacks/:stack/ai/providers', async () => {
    await delay(50);
    return HttpResponse.json([
      {
        id: 'claude',
        label: 'Claude',
        default: true,
        available: true,
        capabilities: { streaming: true, interrupt: true, sessions: true },
      },
    ]);
  }),
  http.post('/api/stacks/:stack/services/:name/diagnose/start', async () => {
    await delay(100);
    return HttpResponse.json({ sessionId: 'sess-storybook', requestId: 'req-storybook', provider: 'claude' });
  }),
  http.post('/api/stacks/:stack/services/:name/diagnose/message', async () => {
    await delay(100);
    return HttpResponse.json({ sessionId: 'sess-storybook', requestId: 'req-storybook', provider: 'claude' });
  }),
  http.post('/api/stacks/:stack/services/:name/diagnose/interrupt', async () => {
    await delay(50);
    return HttpResponse.json({ status: 'interrupted', sessionId: 'sess-storybook', requestId: 'req-storybook' });
  }),
  http.delete('/api/stacks/:stack/services/:name/diagnose/session/:session', async () => {
    await delay(50);
    return HttpResponse.json({ status: 'closed', sessionId: 'sess-storybook' });
  }),
];

// --- All Handlers ---

export const allHandlers = [
  ...statusHandlers,
  ...gitHandlers,
  ...logHandlers,
  ...stackActionHandlers,
  ...serviceActionHandlers,
  ...labConfigHandlers,
  ...configFileHandlers,
  ...cbtOverridesHandlers,
  ...configRegenerateHandlers,
  ...stacksHandlers,
  ...aiHandlers,
];
