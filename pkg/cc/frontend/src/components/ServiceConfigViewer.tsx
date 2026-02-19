import { useState, useEffect } from "react";
import { useAPI } from "../hooks/useAPI";
import type { ConfigFileInfo, ConfigFileContent } from "../types";

interface ServiceConfigViewerProps {
  onToast: (message: string, type: "success" | "error") => void;
}

export default function ServiceConfigViewer({
  onToast,
}: ServiceConfigViewerProps) {
  const { fetchJSON, putJSON, deleteAction } = useAPI();
  const [files, setFiles] = useState<ConfigFileInfo[]>([]);
  const [selectedFile, setSelectedFile] = useState<string | null>(null);
  const [fileContent, setFileContent] = useState<ConfigFileContent | null>(
    null,
  );
  const [editMode, setEditMode] = useState(false);
  const [editContent, setEditContent] = useState("");
  const [saving, setSaving] = useState(false);

  // Load file list
  useEffect(() => {
    fetchJSON<ConfigFileInfo[]>("/api/config/files")
      .then(setFiles)
      .catch((err) => onToast(err.message, "error"));
  }, [fetchJSON, onToast]);

  // Load file content when selected
  useEffect(() => {
    if (!selectedFile) {
      setFileContent(null);

      return;
    }

    fetchJSON<ConfigFileContent>(`/api/config/files/${selectedFile}`)
      .then((data) => {
        setFileContent(data);
        setEditContent(data.overrideContent ?? data.content);
        setEditMode(false);
      })
      .catch((err) => onToast(err.message, "error"));
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
        onToast(
          `Override saved but regen failed: ${resp.regenerateError}`,
          "error",
        );
      } else {
        onToast("Override saved and configs regenerated", "success");
      }

      // Refresh
      const updated = await fetchJSON<ConfigFileContent>(
        `/api/config/files/${selectedFile}`,
      );
      setFileContent(updated);
      setEditMode(false);

      const updatedFiles = await fetchJSON<ConfigFileInfo[]>(
        "/api/config/files",
      );
      setFiles(updatedFiles);
    } catch (err) {
      onToast(
        err instanceof Error ? err.message : "Save failed",
        "error",
      );
    } finally {
      setSaving(false);
    }
  };

  const handleDeleteOverride = async () => {
    if (!selectedFile) return;

    if (!window.confirm("Remove custom override? This will revert to the generated config.")) {
      return;
    }

    try {
      await deleteAction(`/api/config/files/${selectedFile}/override`);
      onToast("Override removed", "success");

      // Refresh
      const updated = await fetchJSON<ConfigFileContent>(
        `/api/config/files/${selectedFile}`,
      );
      setFileContent(updated);
      setEditContent(updated.content);
      setEditMode(false);

      const updatedFiles = await fetchJSON<ConfigFileInfo[]>(
        "/api/config/files",
      );
      setFiles(updatedFiles);
    } catch (err) {
      onToast(
        err instanceof Error ? err.message : "Delete failed",
        "error",
      );
    }
  };

  return (
    <div className="flex gap-4" style={{ height: "calc(100vh - 180px)" }}>
      {/* File list */}
      <div className="flex w-64 shrink-0 flex-col gap-1 overflow-y-auto rounded-xs border border-border bg-surface p-3">
        <div className="mb-2 text-xs/4 font-semibold uppercase tracking-wider text-gray-500">
          Config Files
        </div>
        {files.map((f) => (
          <button
            key={f.name}
            onClick={() => setSelectedFile(f.name)}
            className={`flex items-center gap-2 rounded-xs px-3 py-2 text-left text-sm/5 transition-colors ${
              selectedFile === f.name
                ? "bg-indigo-500/20 text-white"
                : "text-gray-300 hover:bg-white/5"
            }`}
          >
            <span className="min-w-0 flex-1 truncate">{f.name}</span>
            {f.hasOverride && (
              <span className="shrink-0 rounded-xs bg-amber-500/20 px-1.5 py-0.5 text-xs/3 text-amber-400">
                override
              </span>
            )}
          </button>
        ))}
        {files.length === 0 && (
          <div className="text-xs/4 text-gray-600">No config files found</div>
        )}
      </div>

      {/* Content viewer */}
      <div className="flex flex-1 flex-col overflow-hidden rounded-xs border border-border bg-surface">
        {fileContent ? (
          <>
            {/* Toolbar */}
            <div className="flex items-center justify-between border-b border-border px-4 py-2">
              <div className="flex items-center gap-2">
                <span className="text-sm/5 font-medium text-white">
                  {fileContent.name}
                </span>
                {fileContent.hasOverride && (
                  <span className="rounded-xs bg-amber-500/20 px-1.5 py-0.5 text-xs/3 text-amber-400">
                    custom override active
                  </span>
                )}
              </div>
              <div className="flex items-center gap-2">
                {!editMode && (
                  <button
                    onClick={() => {
                      setEditContent(
                        fileContent.overrideContent ?? fileContent.content,
                      );
                      setEditMode(true);
                    }}
                    className="rounded-xs bg-indigo-600/80 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-indigo-500"
                  >
                    {fileContent.hasOverride
                      ? "Edit Override"
                      : "Create Override"}
                  </button>
                )}
                {fileContent.hasOverride && !editMode && (
                  <button
                    onClick={handleDeleteOverride}
                    className="rounded-xs bg-red-600/80 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-red-500"
                  >
                    Remove Override
                  </button>
                )}
                {editMode && (
                  <>
                    <button
                      onClick={handleSaveOverride}
                      disabled={saving}
                      className="rounded-xs bg-emerald-600 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-emerald-500 disabled:opacity-50"
                    >
                      {saving ? "Saving..." : "Save Override"}
                    </button>
                    <button
                      onClick={() => setEditMode(false)}
                      className="rounded-xs bg-gray-600 px-3 py-1 text-xs/4 font-medium text-white transition-colors hover:bg-gray-500"
                    >
                      Cancel
                    </button>
                  </>
                )}
              </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-4">
              {editMode ? (
                <textarea
                  value={editContent}
                  onChange={(e) => setEditContent(e.target.value)}
                  className="size-full resize-none rounded-xs border border-border bg-surface-light p-3 font-mono text-xs/5 text-gray-300 focus:border-indigo-500 focus:outline-hidden"
                  spellCheck={false}
                />
              ) : (
                <pre className="overflow-x-auto whitespace-pre font-mono text-xs/5 text-gray-300">
                  {fileContent.hasOverride
                    ? fileContent.overrideContent
                    : fileContent.content}
                </pre>
              )}
            </div>
          </>
        ) : (
          <div className="flex flex-1 items-center justify-center text-sm/5 text-gray-500">
            Select a config file to view
          </div>
        )}
      </div>
    </div>
  );
}
