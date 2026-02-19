import { useState, useEffect, useMemo } from 'react';
import { useAPI } from '@/hooks/useAPI';
import type { ConfigFileInfo, ConfigFileContent } from '@/types';

interface ServiceConfigViewerProps {
  onToast: (message: string, type: 'success' | 'error') => void;
}

function fileExtColor(name: string): string {
  if (name.endsWith('.yml') || name.endsWith('.yaml')) return 'text-sky-400';
  if (name.endsWith('.json')) return 'text-amber-400';
  if (name.endsWith('.toml')) return 'text-orange-400';

  return 'text-gray-400';
}

export default function ServiceConfigViewer({ onToast }: ServiceConfigViewerProps) {
  const { fetchJSON, putJSON, deleteAction } = useAPI();
  const [files, setFiles] = useState<ConfigFileInfo[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<ConfigFileContent | null>(null);
  const [editMode, setEditMode] = useState(false);
  const [editContent, setEditContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [loading, setLoading] = useState(false);

  // Load file list
  useEffect(() => {
    fetchJSON<ConfigFileInfo[]>('/api/config/files')
      .then(setFiles)
      .catch(err => onToast(err.message, 'error'));
  }, [fetchJSON, onToast]);

  // Load file content when selected
  useEffect(() => {
    if (!selectedFile) {
      setFileContent(null);

      return;
    }

    setLoading(true);

    fetchJSON<ConfigFileContent>(`/api/config/files/${selectedFile}`)
      .then(data => {
        setFileContent(data);
        setEditContent(data.overrideContent ?? data.content);
        setEditMode(false);
      })
      .catch(err => onToast(err.message, 'error'))
      .finally(() => setLoading(false));
  }, [selectedFile, fetchJSON, onToast]);

  const handleSaveOverride = async () => {
    if (!selectedFile) return;

    setSaving(true);

    try {
      const resp = await putJSON<{
        status: string;
        regenerateError?: string;
      }>(`/api/config/files/${selectedFile}/override`, {
        content: editContent,
      });

      if (resp.regenerateError) {
        onToast(`Override saved but regen failed: ${resp.regenerateError}`, 'error');
      } else {
        onToast('Override saved and configs regenerated', 'success');
      }

      // Refresh
      const updated = await fetchJSON<ConfigFileContent>(`/api/config/files/${selectedFile}`);
      setFileContent(updated);
      setEditMode(false);

      const updatedFiles = await fetchJSON<ConfigFileInfo[]>('/api/config/files');
      setFiles(updatedFiles);
    } catch (err) {
      onToast(err instanceof Error ? err.message : 'Save failed', 'error');
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteOverride = async () => {
    if (!selectedFile) return;

    if (!window.confirm('Remove custom override? This will revert to the generated config.')) {
      return;
    }

    try {
      await deleteAction(`/api/config/files/${selectedFile}/override`);
      onToast('Override removed', 'success');

      // Refresh
      const updated = await fetchJSON<ConfigFileContent>(`/api/config/files/${selectedFile}`);
      setFileContent(updated);
      setEditContent(updated.content);
      setEditMode(false);

      const updatedFiles = await fetchJSON<ConfigFileInfo[]>('/api/config/files');
      setFiles(updatedFiles);
    } catch (err) {
      onToast(err instanceof Error ? err.message : 'Delete failed', 'error');
    }
  };

  const overrideCount = files.filter(f => f.hasOverride).length;

  const displayContent = fileContent
    ? ((fileContent.hasOverride ? fileContent.overrideContent : fileContent.content) ?? '')
    : '';

  const lines = useMemo(() => displayContent.split('\n'), [displayContent]);

  return (
    <div className="flex gap-0" style={{ height: 'calc(100dvh - 180px)' }}>
      {/* File list */}
      <div className="flex w-60 shrink-0 flex-col overflow-hidden border-r border-border bg-surface">
        <div className="flex items-center justify-between px-4 py-3">
          <span className="text-xs/4 font-semibold tracking-wider text-gray-500 uppercase">Files</span>
          {overrideCount > 0 && (
            <span className="rounded-full bg-amber-500/15 px-2 py-0.5 text-xs/3 font-medium text-amber-400">
              {overrideCount} override{overrideCount > 1 ? 's' : ''}
            </span>
          )}
        </div>

        <div className="flex flex-1 flex-col gap-px overflow-y-auto px-2 pb-2">
          {files.map(f => {
            const isActive = selectedFile === f.name;

            return (
              <button
                key={f.name}
                onClick={() => setSelectedFile(f.name)}
                className={`group flex items-center gap-2.5 rounded-xs px-3 py-2 text-left transition-colors ${
                  isActive ? 'bg-indigo-500/15 text-white' : 'text-gray-400 hover:bg-white/5 hover:text-gray-200'
                }`}
              >
                <svg
                  className={`size-4 shrink-0 ${isActive ? 'text-indigo-400' : fileExtColor(f.name)}`}
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth={1.5}
                >
                  <path
                    strokeLinecap="round"
                    strokeLinejoin="round"
                    d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m2.25 0H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z"
                  />
                </svg>
                <span className="min-w-0 flex-1 truncate text-sm/5">{f.name}</span>
                {f.hasOverride && <span className="size-1.5 shrink-0 rounded-full bg-amber-400" title="Has override" />}
              </button>
            );
          })}
          {files.length === 0 && (
            <div className="px-3 py-6 text-center text-xs/4 text-gray-600">No config files found</div>
          )}
        </div>
      </div>

      {/* Content viewer */}
      <div className="flex flex-1 flex-col overflow-hidden bg-surface-light/50">
        {loading ? (
          <div className="flex flex-1 items-center justify-center">
            <div className="size-5 animate-spin rounded-full border-2 border-indigo-400 border-t-transparent" />
          </div>
        ) : fileContent ? (
          <>
            {/* Toolbar */}
            <div className="flex items-center justify-between border-b border-border px-4 py-2.5">
              <div className="flex items-center gap-3">
                <span className="font-mono text-sm/5 font-medium text-white">{fileContent.name}</span>
                {fileContent.hasOverride && (
                  <span className="flex items-center gap-1 rounded-full bg-amber-500/15 px-2 py-0.5 text-xs/3 font-medium text-amber-400">
                    <span className="size-1 rounded-full bg-amber-400" />
                    override
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                {!editMode && (
                  <button
                    onClick={() => {
                      setEditContent(fileContent.overrideContent ?? fileContent.content);
                      setEditMode(true);
                    }}
                    className="flex items-center gap-1.5 rounded-xs px-2.5 py-1 text-xs/4 font-medium text-gray-400 transition-colors hover:bg-white/5 hover:text-white"
                  >
                    <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        d="m16.862 4.487 1.687-1.688a1.875 1.875 0 1 1 2.652 2.652L10.582 16.07a4.5 4.5 0 0 1-1.897 1.13L6 18l.8-2.685a4.5 4.5 0 0 1 1.13-1.897l8.932-8.931Zm0 0L19.5 7.125M18 14v4.75A2.25 2.25 0 0 1 15.75 21H5.25A2.25 2.25 0 0 1 3 18.75V8.25A2.25 2.25 0 0 1 5.25 6H10"
                      />
                    </svg>
                    {fileContent.hasOverride ? 'Edit Override' : 'Create Override'}
                  </button>
                )}
                {fileContent.hasOverride && !editMode && (
                  <button
                    onClick={handleDeleteOverride}
                    className="flex items-center gap-1.5 rounded-xs px-2.5 py-1 text-xs/4 font-medium text-gray-500 transition-colors hover:bg-red-500/10 hover:text-red-400"
                  >
                    <svg className="size-3" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth={2}>
                      <path
                        strokeLinecap="round"
                        strokeLinejoin="round"
                        d="m14.74 9-.346 9m-4.788 0L9.26 9m9.968-3.21c.342.052.682.107 1.022.166m-1.022-.165L18.16 19.673a2.25 2.25 0 0 1-2.244 2.077H8.084a2.25 2.25 0 0 1-2.244-2.077L4.772 5.79m14.456 0a48.108 48.108 0 0 0-3.478-.397m-12 .562c.34-.059.68-.114 1.022-.165m0 0a48.11 48.11 0 0 1 3.478-.397m7.5 0v-.916c0-1.18-.91-2.164-2.09-2.201a51.964 51.964 0 0 0-3.32 0c-1.18.037-2.09 1.022-2.09 2.201v.916m7.5 0a48.667 48.667 0 0 0-7.5 0"
                      />
                    </svg>
                    Remove
                  </button>
                )}
                {editMode && (
                  <>
                    <button
                      onClick={handleSaveOverride}
                      disabled={saving}
                      className="rounded-xs bg-indigo-600 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-indigo-500 disabled:opacity-50"
                    >
                      {saving ? 'Saving...' : 'Save Override'}
                    </button>
                    <button
                      onClick={() => setEditMode(false)}
                      className="rounded-xs px-3 py-1 text-xs/4 font-medium text-gray-500 transition-colors hover:text-gray-300"
                    >
                      Cancel
                    </button>
                  </>
                )}
              </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto">
              {editMode ? (
                <textarea
                  value={editContent}
                  onChange={e => setEditContent(e.target.value)}
                  className="size-full resize-none border-none bg-transparent p-4 font-mono text-xs/5 text-gray-300 focus:outline-hidden"
                  spellCheck={false}
                />
              ) : (
                <div className="flex">
                  {/* Line numbers */}
                  <div className="sticky left-0 shrink-0 border-r border-border/50 bg-surface-light/80 py-4 pr-3 pl-4 text-right font-mono text-xs/5 text-gray-700 select-none">
                    {lines.map((_, i) => (
                      <div key={i}>{i + 1}</div>
                    ))}
                  </div>
                  {/* Code */}
                  <pre className="flex-1 overflow-x-auto py-4 pr-6 pl-4 font-mono text-xs/5 text-gray-300">
                    {displayContent}
                  </pre>
                </div>
              )}
            </div>
          </>
        ) : (
          <div className="flex flex-1 flex-col items-center justify-center gap-4">
            <div className="rounded-lg bg-surface-lighter/50 p-4">
              <svg
                className="size-10 text-gray-700"
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                strokeWidth={1}
              >
                <path
                  strokeLinecap="round"
                  strokeLinejoin="round"
                  d="M19.5 14.25v-2.625a3.375 3.375 0 0 0-3.375-3.375h-1.5A1.125 1.125 0 0 1 13.5 7.125v-1.5a3.375 3.375 0 0 0-3.375-3.375H8.25m0 12.75h7.5m-7.5 3H12M10.5 2.25H5.625c-.621 0-1.125.504-1.125 1.125v17.25c0 .621.504 1.125 1.125 1.125h12.75c.621 0 1.125-.504 1.125-1.125V11.25a9 9 0 0 0-9-9Z"
                />
              </svg>
            </div>
            <div className="text-center">
              <p className="text-sm/5 font-medium text-gray-500">No file selected</p>
              <p className="mt-1 text-xs/4 text-gray-700">Choose a config file from the sidebar to view or override</p>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
