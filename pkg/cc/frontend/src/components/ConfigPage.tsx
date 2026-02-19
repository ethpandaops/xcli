import { useState } from "react";
import { useAPI } from "../hooks/useAPI";
import LabConfigEditor from "./LabConfigEditor";
import ServiceConfigViewer from "./ServiceConfigViewer";
import CBTOverridesEditor from "./CBTOverridesEditor";

type Tab = "lab" | "services" | "overrides";

interface ConfigPageProps {
  onBack: () => void;
}

export default function ConfigPage({ onBack }: ConfigPageProps) {
  const { postJSON } = useAPI();
  const [activeTab, setActiveTab] = useState<Tab>("lab");
  const [toast, setToast] = useState<{
    message: string;
    type: "success" | "error";
  } | null>(null);

  const showToast = (message: string, type: "success" | "error") => {
    setToast({ message, type });
    setTimeout(() => setToast(null), 4000);
  };

  const handleRegenerate = async () => {
    try {
      await postJSON<{ status: string }>("/api/config/regenerate");
      showToast("Configs regenerated successfully", "success");
    } catch (err) {
      showToast(
        `Regeneration failed: ${err instanceof Error ? err.message : "Unknown error"}`,
        "error",
      );
    }
  };

  const tabs: { key: Tab; label: string }[] = [
    { key: "lab", label: "Lab Config" },
    { key: "services", label: "Service Configs" },
    { key: "overrides", label: "CBT Overrides" },
  ];

  return (
    <div className="flex h-dvh flex-col bg-bg">
      {/* Header */}
      <header className="flex items-center justify-between border-b border-border bg-surface px-6 py-3">
        <div className="flex items-center gap-4">
          <button
            onClick={onBack}
            className="rounded-xs p-1.5 text-gray-400 transition-colors hover:bg-white/10 hover:text-white"
            title="Back to Dashboard"
          >
            <svg
              className="size-5"
              fill="none"
              viewBox="0 0 24 24"
              strokeWidth={1.5}
              stroke="currentColor"
            >
              <path
                strokeLinecap="round"
                strokeLinejoin="round"
                d="M10.5 19.5 3 12m0 0 7.5-7.5M3 12h18"
              />
            </svg>
          </button>
          <h1 className="text-lg/6 font-bold tracking-tight text-white">
            Config Management
          </h1>
        </div>

        <button
          onClick={handleRegenerate}
          className="rounded-xs bg-indigo-600 px-3 py-1.5 text-sm/5 font-medium text-white transition-colors hover:bg-indigo-500"
        >
          Regenerate Configs
        </button>
      </header>

      {/* Tabs */}
      <div className="border-b border-border bg-surface">
        <div className="flex gap-0 px-6">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`border-b-2 px-4 py-2.5 text-sm/5 font-medium transition-colors ${
                activeTab === tab.key
                  ? "border-indigo-500 text-white"
                  : "border-transparent text-gray-400 hover:text-gray-200"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-y-auto p-6">
        {activeTab === "lab" && <LabConfigEditor onToast={showToast} onNavigateDashboard={onBack} />}
        {activeTab === "services" && (
          <ServiceConfigViewer onToast={showToast} />
        )}
        {activeTab === "overrides" && (
          <CBTOverridesEditor onToast={showToast} />
        )}
      </div>

      {/* Toast */}
      {toast && (
        <div
          className={`fixed bottom-6 right-6 rounded-xs px-4 py-2.5 text-sm/5 font-medium shadow-sm ${
            toast.type === "success"
              ? "bg-emerald-600 text-white"
              : "bg-red-600 text-white"
          }`}
        >
          {toast.message}
        </div>
      )}
    </div>
  );
}
