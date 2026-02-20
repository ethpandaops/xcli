import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { APIError, useAPI } from '@/hooks/useAPI';
import type {
  RedisDeleteManyResponse,
  RedisEncodedValue,
  RedisHashEntry,
  RedisKeyResponse,
  RedisSearchResponse,
  RedisStatusResponse,
  RedisTreeResponse,
  RedisValueMode,
  RedisWriteRequest,
  RedisZSetEntry,
} from '@/types';
import { Group, Panel, Separator } from 'react-resizable-panels';
import type { PanelSize } from 'react-resizable-panels';

type SupportedRedisType = 'string' | 'hash' | 'list' | 'set' | 'zset';
type TTLMode = 'keep' | 'set' | 'clear';

interface RedisPageProps {
  onBack: () => void;
  onNavigateConfig?: () => void;
  stack: string;
}

interface EditableValue {
  mode: RedisValueMode;
  value: string;
}

interface DraftState {
  type: SupportedRedisType;
  stringValue: EditableValue;
  hashEntries: Array<{ field: string; value: EditableValue }>;
  listItems: EditableValue[];
  setMembers: EditableValue[];
  zsetMembers: Array<{ member: EditableValue; score: number }>;
  ttlMode: TTLMode;
  ttlSeconds: number;
}

interface TreeNodeState {
  branches: string[];
  leaves: string[];
  loading: boolean;
  error?: string;
}

const DB_OPTIONS = Array.from({ length: 16 }, (_, i) => i);
const REDIS_KEY_TYPES: SupportedRedisType[] = ['string', 'hash', 'list', 'set', 'zset'];
const redisDbStorageKey = 'xcli:redis-db';
const redisLegacyLayoutStorageKey = 'xcli:redis:panel-layout';
const redisLeftPanelWidthStorageKey = 'xcli:redis:left-panel-width-px';
const redisMiddlePanelWidthStorageKey = 'xcli:redis:middle-panel-width-px';
const redisLeftPanelId = 'redis-tree-panel';
const redisMiddlePanelId = 'redis-keys-panel';
const redisRightPanelId = 'redis-inspector-panel';
const redisLeftPanelDefaultPx = 440;
const redisMiddlePanelDefaultPx = 360;
const redisLeftPanelMinPx = 280;
const redisLeftPanelMaxPx = 760;
const redisMiddlePanelMinPx = 240;
const redisMiddlePanelMaxPx = 700;
const redisRightPanelMinPx = 360;

function clampPanelSize(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function loadPanelWidth(key: string, fallback: number, min: number, max: number): number {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return fallback;
    const parsed = Number.parseFloat(raw);
    if (!Number.isFinite(parsed)) return fallback;
    return clampPanelSize(parsed, min, max);
  } catch {
    return fallback;
  }
}

function persistPanelWidth(key: string, widthPx: number) {
  try {
    localStorage.setItem(key, String(Math.round(widthPx)));
  } catch {
    // ignore storage failures
  }
}

function defaultDraft(type: SupportedRedisType): DraftState {
  return {
    type,
    stringValue: { mode: 'text', value: '' },
    hashEntries: [{ field: '', value: { mode: 'text', value: '' } }],
    listItems: [{ mode: 'text', value: '' }],
    setMembers: [{ mode: 'text', value: '' }],
    zsetMembers: [{ member: { mode: 'text', value: '' }, score: 0 }],
    ttlMode: 'keep',
    ttlSeconds: 3600,
  };
}

function encodedToEditable(value: RedisEncodedValue | undefined): EditableValue {
  if (!value) {
    return { mode: 'text', value: '' };
  }

  return value.mode === 'base64'
    ? { mode: 'base64', value: value.base64 ?? '' }
    : { mode: 'text', value: value.text ?? '' };
}

function editableToEncoded(value: EditableValue): RedisEncodedValue {
  if (value.mode === 'base64') {
    return { mode: 'base64', base64: value.value };
  }

  return { mode: 'text', text: value.value };
}

function ensureSupportedType(value: string): SupportedRedisType {
  if (value === 'string' || value === 'hash' || value === 'list' || value === 'set' || value === 'zset') {
    return value;
  }

  return 'string';
}

function draftFromKey(detail: RedisKeyResponse): DraftState {
  const type = ensureSupportedType(detail.type);

  return {
    type,
    stringValue: encodedToEditable(detail.stringValue),
    hashEntries:
      detail.hashEntries?.map(entry => ({
        field: entry.field,
        value: encodedToEditable(entry.value),
      })) ?? [],
    listItems: detail.listItems?.map(encodedToEditable) ?? [],
    setMembers: detail.setMembers?.map(encodedToEditable) ?? [],
    zsetMembers:
      detail.zsetMembers?.map(entry => ({
        member: encodedToEditable(entry.member),
        score: entry.score,
      })) ?? [],
    ttlMode: 'keep',
    ttlSeconds: detail.ttlMs > 0 ? Math.max(1, Math.ceil(detail.ttlMs / 1000)) : 3600,
  };
}

function formatTTL(ttlMs: number): string {
  if (ttlMs === -1) return 'Persistent';
  if (ttlMs === -2) return 'Missing';
  if (ttlMs <= 0) return 'Expiring';

  const seconds = Math.floor(ttlMs / 1000);
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;

  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${s}s`;
  return `${s}s`;
}

function parseDbFromStorage(): number {
  try {
    const raw = localStorage.getItem(redisDbStorageKey);
    if (!raw) return 0;
    const parsed = Number.parseInt(raw, 10);
    if (Number.isNaN(parsed) || parsed < 0 || parsed > 15) return 0;
    return parsed;
  } catch {
    return 0;
  }
}

function buildWriteRequest(db: number, key: string, version: string, draft: DraftState): RedisWriteRequest {
  const req: RedisWriteRequest = {
    db,
    key,
    type: draft.type,
    expectedVersion: version,
    ttlMode: draft.ttlMode,
  };

  if (draft.ttlMode === 'set') {
    req.ttlSeconds = Math.max(1, Math.floor(draft.ttlSeconds));
  }

  if (draft.type === 'string') {
    req.stringValue = editableToEncoded(draft.stringValue);
  }

  if (draft.type === 'hash') {
    req.hashEntries = draft.hashEntries
      .filter(entry => entry.field.trim() !== '')
      .map(entry => ({ field: entry.field, value: editableToEncoded(entry.value) }));
  }

  if (draft.type === 'list') {
    req.listItems = draft.listItems.map(editableToEncoded);
  }

  if (draft.type === 'set') {
    req.setMembers = draft.setMembers.map(editableToEncoded);
  }

  if (draft.type === 'zset') {
    req.zsetMembers = draft.zsetMembers.map(entry => ({
      member: editableToEncoded(entry.member),
      score: entry.score,
    }));
  }

  return req;
}

function parseCreateJSONPayload(
  type: SupportedRedisType,
  payload: string
): Pick<RedisWriteRequest, 'hashEntries' | 'listItems' | 'setMembers' | 'zsetMembers'> {
  if (type === 'hash') {
    const parsed = JSON.parse(payload) as unknown;
    const entries: RedisHashEntry[] = [];

    if (Array.isArray(parsed)) {
      for (const item of parsed) {
        if (typeof item !== 'object' || item === null) continue;
        const field = String((item as Record<string, unknown>).field ?? '');
        const value = String((item as Record<string, unknown>).value ?? '');
        if (field.trim() === '') continue;

        entries.push({ field, value: { mode: 'text', text: value } });
      }
    } else if (typeof parsed === 'object' && parsed !== null) {
      for (const [field, value] of Object.entries(parsed)) {
        entries.push({
          field,
          value: { mode: 'text', text: typeof value === 'string' ? value : JSON.stringify(value) },
        });
      }
    }

    return { hashEntries: entries };
  }

  if (type === 'list' || type === 'set') {
    const parsed = JSON.parse(payload) as unknown;
    if (!Array.isArray(parsed)) return {};
    const values = parsed.map(item => ({
      mode: 'text' as const,
      text: typeof item === 'string' ? item : JSON.stringify(item),
    }));

    return type === 'list' ? { listItems: values } : { setMembers: values };
  }

  if (type === 'zset') {
    const parsed = JSON.parse(payload) as unknown;
    if (!Array.isArray(parsed)) return {};

    const values: RedisZSetEntry[] = [];
    for (const item of parsed) {
      if (typeof item !== 'object' || item === null) continue;
      const member = (item as Record<string, unknown>).member;
      const score = Number((item as Record<string, unknown>).score ?? 0);
      values.push({
        member: { mode: 'text', text: typeof member === 'string' ? member : JSON.stringify(member) },
        score: Number.isNaN(score) ? 0 : score,
      });
    }

    return { zsetMembers: values };
  }

  return {};
}

function formatPrettyJSON(value: EditableValue): string | null {
  if (value.mode !== 'text') return null;
  const raw = value.value.trim();
  if (raw === '') return null;

  try {
    const parsed = JSON.parse(raw) as unknown;
    if (parsed === null || typeof parsed !== 'object') return null;
    return JSON.stringify(parsed, null, 2);
  } catch {
    return null;
  }
}

type JSONTokenType = 'plain' | 'key' | 'string' | 'number' | 'boolean' | 'null';

function tokenizeJSONLine(line: string): Array<{ text: string; type: JSONTokenType }> {
  const pattern = /("(?:\\u[\dA-Fa-f]{4}|\\[^u]|[^\\"])*"\s*:?|true|false|null|-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?)/g;
  const tokens: Array<{ text: string; type: JSONTokenType }> = [];
  let cursor = 0;

  for (const match of line.matchAll(pattern)) {
    const text = match[0];
    const index = match.index ?? 0;

    if (index > cursor) {
      tokens.push({ text: line.slice(cursor, index), type: 'plain' });
    }

    let type: JSONTokenType = 'number';
    if (text.startsWith('"') && text.endsWith(':')) {
      type = 'key';
    } else if (text.startsWith('"')) {
      type = 'string';
    } else if (text === 'true' || text === 'false') {
      type = 'boolean';
    } else if (text === 'null') {
      type = 'null';
    }

    tokens.push({ text, type });
    cursor = index + text.length;
  }

  if (cursor < line.length) {
    tokens.push({ text: line.slice(cursor), type: 'plain' });
  }

  return tokens;
}

function jsonTokenClassName(type: JSONTokenType): string {
  if (type === 'key') return 'text-info';
  if (type === 'string') return 'text-success';
  if (type === 'number') return 'text-warning';
  if (type === 'boolean') return 'text-accent-light';
  if (type === 'null') return 'text-text-muted italic';
  return 'text-text-secondary';
}

function JSONPreview({ value }: { value: string }) {
  const lines = value.split('\n');

  return (
    <div className="overflow-hidden rounded-xs border border-border/80 bg-linear-to-b from-surface to-surface-light">
      <div className="border-b border-border/70 bg-surface/70 px-3 py-1.5 text-[10px]/3 font-semibold tracking-wider text-text-muted uppercase">
        JSON Preview
      </div>
      <div className="max-h-72 overflow-auto">
        <div className="grid font-mono text-xs/5">
          {lines.map((line, idx) => (
            <div key={idx} className="grid grid-cols-[3rem_1fr]">
              <span className="border-r border-border/60 bg-bg/50 px-2 py-0.5 text-right text-text-disabled select-none">
                {idx + 1}
              </span>
              <span className="px-3 py-0.5 whitespace-pre">
                {line ? (
                  tokenizeJSONLine(line).map((token, tokenIndex) => (
                    <span key={`${idx}-${tokenIndex}`} className={jsonTokenClassName(token.type)}>
                      {token.text}
                    </span>
                  ))
                ) : (
                  <span className="text-text-secondary"> </span>
                )}
              </span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function EncodedInput({
  value,
  onChange,
  placeholder,
  enableJSONPreview = false,
}: {
  value: EditableValue;
  onChange: (next: EditableValue) => void;
  placeholder?: string;
  enableJSONPreview?: boolean;
}) {
  const prettyJSON = enableJSONPreview ? formatPrettyJSON(value) : null;
  const useMultiline =
    value.mode === 'text' && (value.value.includes('\n') || value.value.length > 120 || !!prettyJSON);

  return (
    <div className="flex flex-col gap-2">
      <div className="flex gap-2">
        <select
          value={value.mode}
          onChange={e => onChange({ ...value, mode: e.target.value as RedisValueMode })}
          className="w-24 rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary"
        >
          <option value="text">text</option>
          <option value="base64">base64</option>
        </select>
        {useMultiline ? (
          <textarea
            value={value.value}
            onChange={e => onChange({ ...value, value: e.target.value })}
            placeholder={placeholder}
            className="h-28 flex-1 rounded-xs border border-border bg-surface px-2 py-1 font-mono text-xs/5 text-text-primary focus:border-accent focus:outline-hidden"
          />
        ) : (
          <input
            value={value.value}
            onChange={e => onChange({ ...value, value: e.target.value })}
            placeholder={placeholder}
            className="flex-1 rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-primary focus:border-accent focus:outline-hidden"
          />
        )}
      </div>

      {prettyJSON && <JSONPreview value={prettyJSON} />}
    </div>
  );
}

export default function RedisPage({ onBack, onNavigateConfig, stack }: RedisPageProps) {
  const { fetchJSON, postJSON, putJSON, deleteJSON } = useAPI(stack);
  const redisLeftInitialPxRef = useRef(
    loadPanelWidth(redisLeftPanelWidthStorageKey, redisLeftPanelDefaultPx, redisLeftPanelMinPx, redisLeftPanelMaxPx)
  );
  const redisMiddleInitialPxRef = useRef(
    loadPanelWidth(
      redisMiddlePanelWidthStorageKey,
      redisMiddlePanelDefaultPx,
      redisMiddlePanelMinPx,
      redisMiddlePanelMaxPx
    )
  );
  const [db, setDb] = useState<number>(() => parseDbFromStorage());
  const [status, setStatus] = useState<RedisStatusResponse | null>(null);
  const [statusError, setStatusError] = useState<string | null>(null);
  const [tree, setTree] = useState<Partial<Record<string, TreeNodeState>>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [selectedPrefix, setSelectedPrefix] = useState('');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [search, setSearch] = useState('');
  const [searchResults, setSearchResults] = useState<string[] | null>(null);
  const [searchLoading, setSearchLoading] = useState(false);
  const [detail, setDetail] = useState<RedisKeyResponse | null>(null);
  const [draft, setDraft] = useState<DraftState>(defaultDraft('string'));
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailError, setDetailError] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [toast, setToast] = useState<{ type: 'success' | 'error'; message: string } | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [createKey, setCreateKey] = useState('');
  const [createType, setCreateType] = useState<SupportedRedisType>('string');
  const [createValue, setCreateValue] = useState<EditableValue>({ mode: 'text', value: '' });
  const [createJSON, setCreateJSON] = useState('');
  const [createTTLMode, setCreateTTLMode] = useState<'none' | 'set'>('none');
  const [createTTLSeconds, setCreateTTLSeconds] = useState(3600);
  const [creating, setCreating] = useState(false);
  const toastTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    try {
      localStorage.removeItem(redisLegacyLayoutStorageKey);
    } catch {
      // ignore storage failures
    }
  }, []);

  const showToast = useCallback((message: string, type: 'success' | 'error') => {
    if (toastTimer.current) clearTimeout(toastTimer.current);
    setToast({ message, type });
    toastTimer.current = setTimeout(() => setToast(null), 4000);
  }, []);

  useEffect(() => {
    return () => {
      if (toastTimer.current) clearTimeout(toastTimer.current);
    };
  }, []);

  const loadStatus = useCallback(async () => {
    try {
      const resp = await fetchJSON<RedisStatusResponse>(`/redis/status?db=${db}`);
      setStatus(resp);
      setStatusError(null);
    } catch (err) {
      setStatus(null);
      setStatusError(err instanceof Error ? err.message : 'Failed to load Redis status');
    }
  }, [db, fetchJSON]);

  const loadTree = useCallback(
    async (prefix: string) => {
      setTree(prev => ({
        ...prev,
        [prefix]: {
          branches: prev[prefix]?.branches ?? [],
          leaves: prev[prefix]?.leaves ?? [],
          loading: true,
        },
      }));

      try {
        const resp = await fetchJSON<RedisTreeResponse>(
          `/redis/tree?db=${db}&prefix=${encodeURIComponent(prefix)}&count=250`
        );

        setTree(prev => ({
          ...prev,
          [prefix]: {
            branches: resp.branches,
            leaves: resp.leaves,
            loading: false,
          },
        }));
      } catch (err) {
        setTree(prev => ({
          ...prev,
          [prefix]: {
            branches: prev[prefix]?.branches ?? [],
            leaves: prev[prefix]?.leaves ?? [],
            loading: false,
            error: err instanceof Error ? err.message : 'Failed to load tree',
          },
        }));
      }
    },
    [db, fetchJSON]
  );

  const loadKey = useCallback(
    async (key: string, overwriteDraft: boolean) => {
      setDetailLoading(true);
      setDetailError(null);

      try {
        const resp = await fetchJSON<RedisKeyResponse>(`/redis/key?db=${db}&key=${encodeURIComponent(key)}`);
        setDetail(resp);
        if (overwriteDraft) {
          setDraft(draftFromKey(resp));
          setDirty(false);
        }
      } catch (err) {
        setDetail(null);
        setDetailError(err instanceof Error ? err.message : 'Failed to load key');
      } finally {
        setDetailLoading(false);
      }
    },
    [db, fetchJSON]
  );

  useEffect(() => {
    try {
      localStorage.setItem(redisDbStorageKey, String(db));
    } catch {
      // ignore storage failures
    }
  }, [db]);

  useEffect(() => {
    setTree({});
    setExpanded(new Set());
    setSelectedPrefix('');
    setSelectedKey(null);
    setSelectedKeys(new Set());
    setSearchResults(null);
    setDetail(null);
    setDirty(false);
    setDraft(defaultDraft('string'));
    void loadStatus();
    void loadTree('');
  }, [db, loadStatus, loadTree]);

  useEffect(() => {
    if (!selectedKey) return;
    void loadKey(selectedKey, true);
  }, [selectedKey, loadKey]);

  useEffect(() => {
    const timer = setInterval(() => {
      void loadStatus();
      void loadTree(selectedPrefix);
      if (selectedPrefix !== '') {
        void loadTree('');
      }

      if (selectedKey && !dirty) {
        void loadKey(selectedKey, false);
      }
    }, 5000);

    return () => clearInterval(timer);
  }, [dirty, loadKey, loadStatus, loadTree, selectedKey, selectedPrefix]);

  const currentNode = tree[selectedPrefix];
  const visibleKeys = useMemo(() => {
    if (searchResults) return searchResults;
    return currentNode?.leaves ?? [];
  }, [currentNode?.leaves, searchResults]);

  const toggleBranch = useCallback(
    (prefix: string) => {
      setSelectedPrefix(prefix);
      setExpanded(prev => {
        const next = new Set(prev);
        if (next.has(prefix)) {
          next.delete(prefix);
        } else {
          next.add(prefix);
          const node = tree[prefix];
          if (!node || !node.loading) {
            void loadTree(prefix);
          }
        }
        return next;
      });
    },
    [loadTree, tree]
  );

  const toggleKeySelection = useCallback((key: string) => {
    setSelectedKeys(prev => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
      }

      return next;
    });
  }, []);

  const deleteOneKey = useCallback(
    async (key: string) => {
      if (!window.confirm(`Delete key '${key}'? This cannot be undone.`)) return;

      try {
        await deleteJSON<{ deleted: boolean }>(`/redis/key?db=${db}&key=${encodeURIComponent(key)}`);
        if (selectedKey === key) {
          setSelectedKey(null);
          setDetail(null);
          setDirty(false);
        }

        setSelectedKeys(prev => {
          const next = new Set(prev);
          next.delete(key);
          return next;
        });

        showToast(`Deleted ${key}`, 'success');
        void loadTree(selectedPrefix);
      } catch (err) {
        showToast(err instanceof Error ? err.message : 'Delete failed', 'error');
      }
    },
    [db, deleteJSON, loadTree, selectedKey, selectedPrefix, showToast]
  );

  const deleteSelectedKeys = useCallback(async () => {
    const keys = Array.from(selectedKeys);
    if (keys.length === 0) return;

    if (!window.confirm(`Delete ${keys.length} selected key(s)? This cannot be undone.`)) return;

    try {
      const resp = await postJSON<RedisDeleteManyResponse>('/redis/keys/delete', { db, keys });
      const deletedCount = resp.results.filter(item => item.deleted).length;
      const failedCount = resp.results.length - deletedCount;

      if (selectedKey && keys.includes(selectedKey)) {
        setSelectedKey(null);
        setDetail(null);
        setDirty(false);
      }

      setSelectedKeys(new Set());
      void loadTree(selectedPrefix);
      showToast(
        failedCount > 0 ? `Deleted ${deletedCount} key(s), ${failedCount} failed` : `Deleted ${deletedCount} key(s)`,
        failedCount > 0 ? 'error' : 'success'
      );
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Batch delete failed', 'error');
    }
  }, [db, loadTree, postJSON, selectedKey, selectedKeys, selectedPrefix, showToast]);

  const saveKey = useCallback(async () => {
    if (!detail) return;

    setSaving(true);
    setDetailError(null);

    try {
      const req = buildWriteRequest(db, detail.key, detail.version, draft);
      const updated = await putJSON<RedisKeyResponse>('/redis/key', req);
      setDetail(updated);
      setDraft(draftFromKey(updated));
      setDirty(false);
      showToast(`Saved ${detail.key}`, 'success');
      void loadTree(selectedPrefix);
    } catch (err) {
      if (err instanceof APIError && err.status === 409) {
        const payload = err.body as { current?: RedisKeyResponse };
        if (payload?.current) {
          setDetail(payload.current);
          setDraft(draftFromKey(payload.current));
          setDirty(false);
        }

        setDetailError('Conflict detected. Latest value has been reloaded.');
      } else {
        setDetailError(err instanceof Error ? err.message : 'Save failed');
      }
    } finally {
      setSaving(false);
    }
  }, [db, detail, draft, loadTree, putJSON, selectedPrefix, showToast]);

  const createKeySubmit = useCallback(async () => {
    if (createKey.trim() === '') {
      showToast('Key is required', 'error');
      return;
    }

    setCreating(true);

    try {
      const req: RedisWriteRequest = {
        db,
        key: createKey.trim(),
        type: createType,
        ttlMode: createTTLMode,
      };

      if (createTTLMode === 'set') {
        req.ttlSeconds = Math.max(1, Math.floor(createTTLSeconds));
      }

      if (createType === 'string') {
        req.stringValue = editableToEncoded(createValue);
      } else {
        const parsed = parseCreateJSONPayload(createType, createJSON);
        Object.assign(req, parsed);
      }

      const created = await postJSON<RedisKeyResponse>('/redis/key', req);
      setCreateOpen(false);
      setCreateKey('');
      setCreateValue({ mode: 'text', value: '' });
      setCreateJSON('');
      showToast(`Created ${created.key}`, 'success');

      setSelectedKey(created.key);
      setSelectedPrefix('');
      void loadTree('');
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to create key', 'error');
    } finally {
      setCreating(false);
    }
  }, [
    createJSON,
    createKey,
    createTTLMode,
    createTTLSeconds,
    createType,
    createValue,
    db,
    loadTree,
    postJSON,
    showToast,
  ]);

  const runSearch = useCallback(async () => {
    if (search.trim() === '') {
      setSearchResults(null);
      return;
    }

    setSearchLoading(true);

    try {
      const resp = await fetchJSON<RedisSearchResponse>(
        `/redis/keys/search?db=${db}&q=${encodeURIComponent(search.trim())}&count=250`
      );
      setSearchResults(resp.keys);
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Search failed', 'error');
    } finally {
      setSearchLoading(false);
    }
  }, [db, fetchJSON, search, showToast]);

  const handleLeftPanelResize = useCallback((panelSize: PanelSize) => {
    persistPanelWidth(
      redisLeftPanelWidthStorageKey,
      clampPanelSize(panelSize.inPixels, redisLeftPanelMinPx, redisLeftPanelMaxPx)
    );
  }, []);

  const handleMiddlePanelResize = useCallback((panelSize: PanelSize) => {
    persistPanelWidth(
      redisMiddlePanelWidthStorageKey,
      clampPanelSize(panelSize.inPixels, redisMiddlePanelMinPx, redisMiddlePanelMaxPx)
    );
  }, []);

  const renderBranch = (prefix: string, depth: number) => {
    const node = tree[prefix];
    const isLoading = node?.loading;

    return (
      <div key={prefix} className="flex flex-col">
        {isLoading && prefix !== '' && (
          <div className="px-2 py-1 text-xs/4 text-text-muted" style={{ paddingLeft: `${depth * 16 + 8}px` }}>
            loading...
          </div>
        )}

        {node?.branches.map(branch => {
          const isOpen = expanded.has(branch);
          return (
            <div key={branch} className="flex flex-col">
              <button
                onClick={() => toggleBranch(branch)}
                className={`flex items-center gap-2 rounded-xs px-2 py-1 text-left text-xs/4 transition-colors ${
                  selectedPrefix === branch ? 'bg-accent/20 text-accent-light' : 'text-text-secondary hover:bg-surface'
                }`}
                style={{ paddingLeft: `${depth * 16 + 8}px` }}
              >
                <span className="font-mono text-[10px]">{isOpen ? '▼' : '▶'}</span>
                <span className="font-mono">{branch}</span>
              </button>
              {isOpen && renderBranch(branch, depth + 1)}
            </div>
          );
        })}

        {node?.leaves.map(leaf => (
          <button
            key={leaf}
            onClick={() => setSelectedKey(leaf)}
            className={`rounded-xs px-2 py-1 text-left font-mono text-xs/4 transition-colors ${
              selectedKey === leaf ? 'bg-success/20 text-success' : 'text-text-muted hover:bg-surface'
            }`}
            style={{ paddingLeft: `${depth * 16 + 28}px` }}
          >
            {leaf}
          </button>
        ))}
      </div>
    );
  };

  const renderTreeSection = () => (
    <section className="flex min-h-0 flex-col rounded-sm border border-border bg-surface-light">
      <div className="border-b border-border p-3">
        <div className="mb-2 flex items-center justify-between">
          <h2 className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Tree</h2>
          <button
            onClick={() => {
              void loadStatus();
              void loadTree(selectedPrefix);
            }}
            className="rounded-xs border border-border px-2 py-1 text-[10px]/3 text-text-muted hover:text-text-secondary"
          >
            Refresh
          </button>
        </div>

        <div className="mb-2 flex items-center gap-2">
          <label className="text-xs/4 text-text-muted">DB</label>
          <select
            value={db}
            onChange={e => setDb(Number.parseInt(e.target.value, 10) || 0)}
            className="rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary"
          >
            {DB_OPTIONS.map(value => (
              <option key={value} value={value}>
                {value}
              </option>
            ))}
          </select>
          <span className="font-mono text-[10px]/3 text-text-disabled">
            {status ? `${status.addr} • ${status.dbSize} keys` : (statusError ?? 'disconnected')}
          </span>
        </div>

        <div className="flex gap-2">
          <input
            value={search}
            onChange={e => setSearch(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Enter') {
                e.preventDefault();
                void runSearch();
              }
            }}
            placeholder="Search keys..."
            className="flex-1 rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-primary focus:border-accent focus:outline-hidden"
          />
          <button
            onClick={() => void runSearch()}
            disabled={searchLoading}
            className="rounded-xs border border-border px-2 py-1 text-xs/4 text-text-secondary hover:text-text-primary disabled:opacity-50"
          >
            {searchLoading ? '...' : 'Go'}
          </button>
          {searchResults && (
            <button
              onClick={() => setSearchResults(null)}
              className="rounded-xs border border-border px-2 py-1 text-xs/4 text-text-muted hover:text-text-secondary"
            >
              Clear
            </button>
          )}
        </div>
      </div>

      <div className="flex-1 overflow-auto p-2">{renderBranch('', 0)}</div>
    </section>
  );

  const renderKeysSection = () => (
    <section className="flex min-h-0 flex-col rounded-sm border border-border bg-surface-light">
      <div className="border-b border-border p-3">
        <h2 className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">
          {searchResults ? `Search (${searchResults.length})` : `Keys (${visibleKeys.length})`}
        </h2>
        <p className="mt-1 font-mono text-[10px]/3 text-text-disabled">
          {searchResults ? `query: ${search}` : `prefix: ${selectedPrefix || '(root)'}`}
        </p>
      </div>
      <div className="flex-1 overflow-auto p-2">
        {visibleKeys.map(key => (
          <div
            key={key}
            className={`mb-1 flex items-center gap-2 rounded-xs border px-2 py-1 ${
              selectedKey === key ? 'border-success/40 bg-success/10' : 'border-border bg-surface'
            }`}
          >
            <input type="checkbox" checked={selectedKeys.has(key)} onChange={() => toggleKeySelection(key)} />
            <button
              onClick={() => setSelectedKey(key)}
              className="flex-1 truncate text-left font-mono text-xs/4 text-text-secondary hover:text-text-primary"
            >
              {key}
            </button>
            <button onClick={() => void deleteOneKey(key)} className="text-xs/4 text-error hover:text-error/80">
              Delete
            </button>
          </div>
        ))}
        {visibleKeys.length === 0 && (
          <div className="rounded-xs border border-border/60 bg-surface p-3 text-xs/4 text-text-muted">
            No keys in this view.
          </div>
        )}
      </div>

      {selectedKeys.size > 0 && (
        <div className="border-t border-border p-2">
          <button
            onClick={() => void deleteSelectedKeys()}
            className="w-full rounded-xs bg-error/15 px-3 py-2 text-xs/4 font-medium text-error hover:bg-error/25"
          >
            Delete {selectedKeys.size} Selected Key(s)
          </button>
        </div>
      )}
    </section>
  );

  const renderInspectorSection = () => (
    <section className="flex min-h-0 flex-col rounded-sm border border-border bg-surface-light">
      <div className="border-b border-border p-3">
        <h2 className="text-xs/4 font-semibold tracking-wider text-text-muted uppercase">Inspector</h2>
        {detail && (
          <p className="mt-1 font-mono text-[10px]/3 text-text-disabled">
            {detail.key} • {detail.type} • TTL {formatTTL(detail.ttlMs)}
          </p>
        )}
      </div>

      <div className="flex-1 overflow-auto p-3">
        {!selectedKey && (
          <div className="rounded-xs border border-border/60 bg-surface p-3 text-xs/4 text-text-muted">
            Select a key to inspect and edit.
          </div>
        )}

        {selectedKey && detailLoading && <p className="text-xs/4 text-text-muted">Loading key...</p>}
        {selectedKey && detailError && <p className="text-xs/4 text-error">{detailError}</p>}

        {detail && (
          <div className="flex flex-col gap-3">
            <div className="grid grid-cols-3 gap-2">
              <div>
                <label className="mb-1 block text-[10px]/3 text-text-disabled uppercase">Type</label>
                <input
                  value={draft.type}
                  disabled
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-muted"
                />
              </div>
              <div>
                <label className="mb-1 block text-[10px]/3 text-text-disabled uppercase">TTL Mode</label>
                <select
                  value={draft.ttlMode}
                  onChange={e => {
                    setDraft(prev => ({ ...prev, ttlMode: e.target.value as TTLMode }));
                    setDirty(true);
                  }}
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary"
                >
                  <option value="keep">Keep</option>
                  <option value="set">Set</option>
                  <option value="clear">Clear</option>
                </select>
              </div>
              <div>
                <label className="mb-1 block text-[10px]/3 text-text-disabled uppercase">TTL Seconds</label>
                <input
                  type="number"
                  value={draft.ttlSeconds}
                  disabled={draft.ttlMode !== 'set'}
                  onChange={e => {
                    const next = Number.parseInt(e.target.value, 10) || 1;
                    setDraft(prev => ({ ...prev, ttlSeconds: next }));
                    setDirty(true);
                  }}
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary disabled:opacity-50"
                />
              </div>
            </div>

            {draft.type === 'string' && (
              <EncodedInput
                value={draft.stringValue}
                onChange={next => {
                  setDraft(prev => ({ ...prev, stringValue: next }));
                  setDirty(true);
                }}
                placeholder="String value"
                enableJSONPreview
              />
            )}

            {draft.type === 'hash' && (
              <div className="flex flex-col gap-2">
                {draft.hashEntries.map((entry, index) => (
                  <div key={`${entry.field}-${index}`} className="grid grid-cols-[10rem_1fr_auto] gap-2">
                    <input
                      value={entry.field}
                      onChange={e => {
                        const next = [...draft.hashEntries];
                        next[index] = { ...next[index], field: e.target.value };
                        setDraft(prev => ({ ...prev, hashEntries: next }));
                        setDirty(true);
                      }}
                      placeholder="field"
                      className="rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-primary"
                    />
                    <EncodedInput
                      value={entry.value}
                      onChange={nextValue => {
                        const next = [...draft.hashEntries];
                        next[index] = { ...next[index], value: nextValue };
                        setDraft(prev => ({ ...prev, hashEntries: next }));
                        setDirty(true);
                      }}
                      placeholder="value"
                    />
                    <button
                      onClick={() => {
                        const next = draft.hashEntries.filter((_, i) => i !== index);
                        setDraft(prev => ({ ...prev, hashEntries: next }));
                        setDirty(true);
                      }}
                      className="rounded-xs border border-border px-2 py-1 text-xs/4 text-error"
                    >
                      x
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => {
                    setDraft(prev => ({
                      ...prev,
                      hashEntries: [...prev.hashEntries, { field: '', value: { mode: 'text', value: '' } }],
                    }));
                    setDirty(true);
                  }}
                  className="self-start rounded-xs border border-border px-2 py-1 text-xs/4 text-text-secondary"
                >
                  Add Field
                </button>
              </div>
            )}

            {draft.type === 'list' && (
              <div className="flex flex-col gap-2">
                {draft.listItems.map((item, index) => (
                  <div key={index} className="grid grid-cols-[auto_1fr_auto] gap-2">
                    <span className="w-6 pt-1 text-right font-mono text-[10px]/3 text-text-disabled">{index}</span>
                    <EncodedInput
                      value={item}
                      onChange={nextItem => {
                        const next = [...draft.listItems];
                        next[index] = nextItem;
                        setDraft(prev => ({ ...prev, listItems: next }));
                        setDirty(true);
                      }}
                    />
                    <button
                      onClick={() => {
                        const next = draft.listItems.filter((_, i) => i !== index);
                        setDraft(prev => ({ ...prev, listItems: next }));
                        setDirty(true);
                      }}
                      className="rounded-xs border border-border px-2 py-1 text-xs/4 text-error"
                    >
                      x
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => {
                    setDraft(prev => ({ ...prev, listItems: [...prev.listItems, { mode: 'text', value: '' }] }));
                    setDirty(true);
                  }}
                  className="self-start rounded-xs border border-border px-2 py-1 text-xs/4 text-text-secondary"
                >
                  Add Item
                </button>
              </div>
            )}

            {draft.type === 'set' && (
              <div className="flex flex-col gap-2">
                {draft.setMembers.map((member, index) => (
                  <div key={index} className="grid grid-cols-[1fr_auto] gap-2">
                    <EncodedInput
                      value={member}
                      onChange={nextMember => {
                        const next = [...draft.setMembers];
                        next[index] = nextMember;
                        setDraft(prev => ({ ...prev, setMembers: next }));
                        setDirty(true);
                      }}
                    />
                    <button
                      onClick={() => {
                        const next = draft.setMembers.filter((_, i) => i !== index);
                        setDraft(prev => ({ ...prev, setMembers: next }));
                        setDirty(true);
                      }}
                      className="rounded-xs border border-border px-2 py-1 text-xs/4 text-error"
                    >
                      x
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => {
                    setDraft(prev => ({ ...prev, setMembers: [...prev.setMembers, { mode: 'text', value: '' }] }));
                    setDirty(true);
                  }}
                  className="self-start rounded-xs border border-border px-2 py-1 text-xs/4 text-text-secondary"
                >
                  Add Member
                </button>
              </div>
            )}

            {draft.type === 'zset' && (
              <div className="flex flex-col gap-2">
                {draft.zsetMembers.map((entry, index) => (
                  <div key={index} className="grid grid-cols-[1fr_7rem_auto] gap-2">
                    <EncodedInput
                      value={entry.member}
                      onChange={nextMember => {
                        const next = [...draft.zsetMembers];
                        next[index] = { ...next[index], member: nextMember };
                        setDraft(prev => ({ ...prev, zsetMembers: next }));
                        setDirty(true);
                      }}
                      placeholder="member"
                    />
                    <input
                      type="number"
                      value={entry.score}
                      onChange={e => {
                        const score = Number.parseFloat(e.target.value);
                        const next = [...draft.zsetMembers];
                        next[index] = { ...next[index], score: Number.isNaN(score) ? 0 : score };
                        setDraft(prev => ({ ...prev, zsetMembers: next }));
                        setDirty(true);
                      }}
                      className="rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-primary"
                    />
                    <button
                      onClick={() => {
                        const next = draft.zsetMembers.filter((_, i) => i !== index);
                        setDraft(prev => ({ ...prev, zsetMembers: next }));
                        setDirty(true);
                      }}
                      className="rounded-xs border border-border px-2 py-1 text-xs/4 text-error"
                    >
                      x
                    </button>
                  </div>
                ))}
                <button
                  onClick={() => {
                    setDraft(prev => ({
                      ...prev,
                      zsetMembers: [...prev.zsetMembers, { member: { mode: 'text', value: '' }, score: 0 }],
                    }));
                    setDirty(true);
                  }}
                  className="self-start rounded-xs border border-border px-2 py-1 text-xs/4 text-text-secondary"
                >
                  Add Member
                </button>
              </div>
            )}
          </div>
        )}
      </div>

      {detail && (
        <div className="border-t border-border p-3">
          <div className="flex items-center gap-2">
            <button
              onClick={() => void saveKey()}
              disabled={!dirty || saving}
              className="rounded-xs bg-accent px-3 py-1.5 text-xs/4 font-medium text-text-primary disabled:opacity-50"
            >
              {saving ? 'Saving...' : 'Save'}
            </button>
            <button
              onClick={() => {
                setDraft(draftFromKey(detail));
                setDirty(false);
              }}
              disabled={!dirty}
              className="rounded-xs border border-border px-3 py-1.5 text-xs/4 text-text-secondary disabled:opacity-50"
            >
              Reset
            </button>
            <button
              onClick={() => void deleteOneKey(detail.key)}
              className="rounded-xs border border-error/40 bg-error/10 px-3 py-1.5 text-xs/4 text-error"
            >
              Delete
            </button>
          </div>
        </div>
      )}
    </section>
  );

  return (
    <div className="flex h-dvh flex-col bg-bg">
      <header className="flex items-center justify-between border-b border-border bg-surface px-6 py-3">
        <div className="flex items-center gap-3">
          <button
            onClick={onBack}
            className="group flex items-center gap-2 rounded-xs py-1 pr-2 text-text-tertiary transition-colors hover:text-text-primary"
          >
            <svg
              className="size-4 transition-transform group-hover:-translate-x-0.5"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={2}
              stroke="currentColor"
            >
              <path strokeLinecap="round" strokeLinejoin="round" d="M10.5 19.5 3 12m0 0 7.5-7.5M3 12h18" />
            </svg>
            <span className="text-sm/5 font-medium">Dashboard</span>
          </button>
          <span className="text-border">{'/'}</span>
          <h1 className="text-sm/5 font-semibold text-text-primary">Redis Explorer</h1>
        </div>

        <div className="flex items-center gap-2">
          {onNavigateConfig && (
            <button
              onClick={onNavigateConfig}
              className="rounded-xs border border-border px-3 py-1.5 text-xs/4 font-medium text-text-secondary transition-colors hover:border-text-muted hover:text-text-primary"
            >
              Config
            </button>
          )}
          <button
            onClick={() => setCreateOpen(true)}
            className="rounded-xs bg-accent px-3 py-1.5 text-xs/4 font-medium text-text-primary transition-colors hover:bg-accent-light"
          >
            Add Key
          </button>
        </div>
      </header>

      <div className="hidden flex-1 overflow-hidden p-3 xl:flex">
        <Group orientation="horizontal" resizeTargetMinimumSize={{ fine: 20, coarse: 28 }}>
          <Panel
            id={redisLeftPanelId}
            defaultSize={redisLeftInitialPxRef.current}
            minSize={redisLeftPanelMinPx}
            maxSize={redisLeftPanelMaxPx}
            onResize={handleLeftPanelResize}
          >
            <div className="h-full min-w-0 pr-1.5">{renderTreeSection()}</div>
          </Panel>
          <Separator className="cc-resize-handle" />
          <Panel
            id={redisMiddlePanelId}
            defaultSize={redisMiddleInitialPxRef.current}
            minSize={redisMiddlePanelMinPx}
            maxSize={redisMiddlePanelMaxPx}
            onResize={handleMiddlePanelResize}
          >
            <div className="h-full min-w-0 px-1.5">{renderKeysSection()}</div>
          </Panel>
          <Separator className="cc-resize-handle" />
          <Panel id={redisRightPanelId} minSize={redisRightPanelMinPx}>
            <div className="h-full min-w-0 pl-1.5">{renderInspectorSection()}</div>
          </Panel>
        </Group>
      </div>

      <div className="grid flex-1 grid-cols-1 gap-3 overflow-hidden p-3 xl:hidden">
        {renderTreeSection()}
        {renderKeysSection()}
        {renderInspectorSection()}
      </div>

      {createOpen && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-overlay/60">
          <div className="w-full max-w-2xl rounded-sm border border-border bg-surface-light p-5">
            <h3 className="mb-3 text-sm/5 font-semibold text-text-primary">Create Redis Key</h3>
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1 block text-xs/4 text-text-muted">Key</label>
                <input
                  value={createKey}
                  onChange={e => setCreateKey(e.target.value)}
                  placeholder="cbt:external:example"
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-primary"
                />
              </div>
              <div>
                <label className="mb-1 block text-xs/4 text-text-muted">Type</label>
                <select
                  value={createType}
                  onChange={e => setCreateType(e.target.value as SupportedRedisType)}
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary"
                >
                  {REDIS_KEY_TYPES.map(type => (
                    <option key={type} value={type}>
                      {type}
                    </option>
                  ))}
                </select>
              </div>
            </div>

            <div className="mt-3 grid grid-cols-2 gap-3">
              <div>
                <label className="mb-1 block text-xs/4 text-text-muted">TTL</label>
                <select
                  value={createTTLMode}
                  onChange={e => setCreateTTLMode(e.target.value as 'none' | 'set')}
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary"
                >
                  <option value="none">No TTL</option>
                  <option value="set">Set TTL</option>
                </select>
              </div>
              <div>
                <label className="mb-1 block text-xs/4 text-text-muted">TTL Seconds</label>
                <input
                  type="number"
                  value={createTTLSeconds}
                  disabled={createTTLMode !== 'set'}
                  onChange={e => setCreateTTLSeconds(Number.parseInt(e.target.value, 10) || 1)}
                  className="w-full rounded-xs border border-border bg-surface px-2 py-1 text-xs/4 text-text-secondary disabled:opacity-50"
                />
              </div>
            </div>

            <div className="mt-3">
              <label className="mb-1 block text-xs/4 text-text-muted">Value</label>
              {createType === 'string' ? (
                <EncodedInput value={createValue} onChange={setCreateValue} placeholder="string value" />
              ) : (
                <>
                  <textarea
                    value={createJSON}
                    onChange={e => setCreateJSON(e.target.value)}
                    placeholder={
                      createType === 'hash'
                        ? '{"field":"value"} or [{"field":"a","value":"b"}]'
                        : createType === 'zset'
                          ? '[{"member":"a","score":1}]'
                          : '["item1","item2"]'
                    }
                    className="h-32 w-full rounded-xs border border-border bg-surface px-2 py-1 font-mono text-xs/5 text-text-primary"
                  />
                  <p className="mt-1 text-[10px]/3 text-text-disabled">
                    Non-string create uses JSON payload for speed. You can fine-tune item-level edits after creation.
                  </p>
                </>
              )}
            </div>

            <div className="mt-4 flex justify-end gap-2">
              <button
                onClick={() => setCreateOpen(false)}
                className="rounded-xs border border-border px-3 py-1.5 text-xs/4 text-text-muted"
              >
                Cancel
              </button>
              <button
                onClick={() => void createKeySubmit()}
                disabled={creating}
                className="rounded-xs bg-accent px-3 py-1.5 text-xs/4 font-medium text-text-primary disabled:opacity-50"
              >
                {creating ? 'Creating...' : 'Create'}
              </button>
            </div>
          </div>
        </div>
      )}

      {toast && (
        <div
          className={`fixed right-6 bottom-6 rounded-sm px-4 py-2 text-sm/5 font-medium shadow-lg ${
            toast.type === 'success'
              ? 'bg-success/15 text-success ring-1 ring-success/25'
              : 'bg-error/15 text-error ring-1 ring-error/25'
          }`}
        >
          {toast.message}
        </div>
      )}
    </div>
  );
}
