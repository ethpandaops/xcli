import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import type { Components } from 'react-markdown';

const components: Components = {
  h1: ({ children }) => <h1 className="mt-4 mb-2 text-base/6 font-bold text-text-primary">{children}</h1>,
  h2: ({ children }) => <h2 className="mt-3 mb-2 text-sm/5 font-bold text-text-primary">{children}</h2>,
  h3: ({ children }) => <h3 className="mt-2 mb-1 text-sm/5 font-semibold text-text-primary">{children}</h3>,
  h4: ({ children }) => <h4 className="mt-2 mb-1 text-xs/4 font-semibold text-text-primary">{children}</h4>,
  p: ({ children }) => <p className="mb-2 text-sm/6 text-text-secondary">{children}</p>,
  ul: ({ children }) => (
    <ul className="mb-2 flex flex-col gap-0.5 pl-5" style={{ listStyleType: 'disc' }}>
      {children}
    </ul>
  ),
  ol: ({ children }) => (
    <ol className="mb-2 flex flex-col gap-0.5 pl-5" style={{ listStyleType: 'decimal' }}>
      {children}
    </ol>
  ),
  li: ({ children }) => <li className="text-sm/5 text-text-secondary">{children}</li>,
  a: ({ href, children }) => (
    <a
      href={href}
      target="_blank"
      rel="noopener noreferrer"
      className="text-accent-light underline hover:text-accent-light/80"
    >
      {children}
    </a>
  ),
  strong: ({ children }) => <strong className="font-semibold text-text-primary">{children}</strong>,
  em: ({ children }) => <em className="text-text-secondary italic">{children}</em>,
  code: ({ children, className }) => {
    const isBlock = className?.includes('language-');
    if (isBlock) {
      return <code className="block text-xs/5 text-text-secondary">{children}</code>;
    }
    return (
      <code className="rounded-xs bg-surface-light px-1 py-0.5 font-mono text-xs text-accent-light">{children}</code>
    );
  },
  pre: ({ children }) => (
    <pre className="mb-2 overflow-x-auto rounded-xs border border-border/50 bg-surface-light px-3 py-2 font-mono text-xs/5 whitespace-pre-wrap">
      {children}
    </pre>
  ),
  blockquote: ({ children }) => (
    <blockquote className="mb-2 border-l-2 border-accent/40 pl-3 text-sm/5 text-text-muted italic">
      {children}
    </blockquote>
  ),
  hr: () => <hr className="my-3 border-border/40" />,
  table: ({ children }) => (
    <div className="mb-2 overflow-x-auto">
      <table className="w-full text-sm/5">{children}</table>
    </div>
  ),
  thead: ({ children }) => <thead className="border-b border-border text-text-primary">{children}</thead>,
  th: ({ children }) => <th className="px-2 py-1 text-left text-xs/4 font-semibold">{children}</th>,
  td: ({ children }) => <td className="border-t border-border/30 px-2 py-1 text-text-secondary">{children}</td>,
};

interface MarkdownProps {
  children: string;
  className?: string;
}

export default function Markdown({ children, className }: MarkdownProps) {
  return (
    <div className={className}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} components={components}>
        {children}
      </ReactMarkdown>
    </div>
  );
}
