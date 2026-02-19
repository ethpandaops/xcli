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
  http.get('/api/status', async () => {
    await delay(100);
    return HttpResponse.json(mockStatusResponse);
  }),
  http.get('/api/stack/status', async () => {
    await delay(100);
    return HttpResponse.json(mockStackStatus);
  }),
];

// --- Git ---

export const gitHandlers = [
  http.get('/api/git', async () => {
    await delay(100);
    return HttpResponse.json(mockGitResponse);
  }),
];

// --- Logs ---

export const logHandlers = [
  http.get('/api/logs', async () => {
    await delay(100);
    return HttpResponse.json(mockLogs);
  }),
  http.get('/api/services/:name/logs', async () => {
    await delay(100);
    return HttpResponse.json(mockLogs.slice(0, 5));
  }),
];

// --- Stack Actions ---

export const stackActionHandlers = [
  http.post('/api/stack/up', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stack/down', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stack/cancel', async () => {
    await delay(100);
    return HttpResponse.json({ status: 'ok' });
  }),
  http.post('/api/stack/restart', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'ok' });
  }),
];

// --- Service Actions ---

export const serviceActionHandlers = [
  http.post('/api/services/:name/:action', async () => {
    await delay(500);
    return HttpResponse.json({ status: 'ok' });
  }),
];

// --- Lab Config ---

export const labConfigHandlers = [
  http.get('/api/config/lab', async () => {
    await delay(150);
    return HttpResponse.json(mockLabConfig);
  }),
  http.put('/api/config/lab', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  ...statusHandlers,
  ...stackActionHandlers,
];

// --- Config Files ---

export const configFileHandlers = [
  http.get('/api/config/files', async () => {
    await delay(100);
    return HttpResponse.json(mockConfigFiles);
  }),
  http.get('/api/config/files/:name', async () => {
    await delay(150);
    return HttpResponse.json(mockConfigFileContent);
  }),
  http.put('/api/config/files/:name/override', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  http.delete('/api/config/files/:name/override', async () => {
    await delay(200);
    return HttpResponse.json({ status: 'deleted' });
  }),
];

// --- CBT Overrides ---

export const cbtOverridesHandlers = [
  http.get('/api/config/overrides', async () => {
    await delay(150);
    return HttpResponse.json(mockCBTOverrides);
  }),
  http.get('/api/config', async () => {
    await delay(100);
    return HttpResponse.json({ mode: mockConfig.mode });
  }),
  http.put('/api/config/overrides', async () => {
    await delay(300);
    return HttpResponse.json({ status: 'saved' });
  }),
  http.get('/api/services', async () => {
    await delay(100);
    return HttpResponse.json(mockStatusResponse.services);
  }),
  ...serviceActionHandlers,
  ...statusHandlers,
];

// --- Config Regenerate ---

export const configRegenerateHandlers = [
  http.post('/api/config/regenerate', async () => {
    await delay(500);
    return HttpResponse.json({ status: 'ok' });
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
];
