interface SpinnerProps {
  text?: string;
  size?: "sm" | "md";
  centered?: boolean;
}

export default function Spinner({ text = "Loading", size = "sm", centered = false }: SpinnerProps) {
  if (centered) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-5 py-16">
        <div className="flex items-center gap-1.5">
          <span className="size-1.5 animate-pulse rounded-full bg-indigo-400/80 [animation-delay:0ms]" />
          <span className="size-1.5 animate-pulse rounded-full bg-indigo-400/80 [animation-delay:150ms]" />
          <span className="size-1.5 animate-pulse rounded-full bg-indigo-400/80 [animation-delay:300ms]" />
        </div>
        {text && (
          <span className="text-xs/4 font-medium tracking-wide text-gray-600">
            {text}
          </span>
        )}
      </div>
    );
  }

  const sizeClass = size === "sm" ? "size-3.5" : "size-4";
  const textClass = size === "sm" ? "text-xs/4" : "text-sm/5";

  return (
    <span className={`inline-flex items-center gap-2 ${textClass} text-gray-500`}>
      <svg
        className={`${sizeClass} animate-spin`}
        fill="none"
        viewBox="0 0 24 24"
      >
        <circle
          className="opacity-15"
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          strokeWidth="3"
        />
        <path
          className="opacity-60"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
        />
      </svg>
      {text}
    </span>
  );
}
