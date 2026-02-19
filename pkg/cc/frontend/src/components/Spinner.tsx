interface SpinnerProps {
  text?: string;
  size?: "sm" | "md";
  centered?: boolean;
}

export default function Spinner({ text = "Loading", size = "sm", centered = false }: SpinnerProps) {
  const sizeClass = size === "sm" ? "size-4" : "size-5";
  const textClass = size === "sm" ? "text-xs/4" : "text-sm";

  const spinner = (
    <span className={`inline-flex items-center gap-2.5 ${textClass} text-gray-500`}>
      <svg
        className={`${sizeClass} animate-spin`}
        fill="none"
        viewBox="0 0 24 24"
      >
        <circle
          className="opacity-20"
          cx="12"
          cy="12"
          r="10"
          stroke="currentColor"
          strokeWidth="3"
        />
        <path
          className="opacity-75"
          fill="currentColor"
          d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"
        />
      </svg>
      {text}
    </span>
  );

  if (centered) {
    return (
      <div className="flex h-full items-center justify-center py-16">
        {spinner}
      </div>
    );
  }

  return spinner;
}
