import type { RepoInfo } from '@/types';
import Spinner from '@/components/Spinner';

interface GitStatusProps {
  repos: RepoInfo[];
}

export default function GitStatus({ repos }: GitStatusProps) {
  if (repos.length === 0) return <Spinner />;

  return (
    <div className="flex flex-col gap-2">
      {repos.map(repo => (
        <div key={repo.name} className="rounded-xs bg-surface px-3 py-2 text-xs/4">
          <div className="flex items-center justify-between">
            <span className="font-medium text-text-secondary">{repo.name}</span>
            <span className="font-mono text-text-muted">{repo.branch}</span>
          </div>
          <div className="mt-1 flex gap-2">
            {repo.behindBy > 0 && <span className="text-warning">{repo.behindBy} behind</span>}
            {repo.aheadBy > 0 && <span className="text-info">{repo.aheadBy} ahead</span>}
            {repo.hasUncommitted && <span className="text-warning">{repo.uncommittedCount} uncommitted</span>}
            {repo.isUpToDate && !repo.hasUncommitted && !repo.error && <span className="text-success">up to date</span>}
            {repo.error && <span className="text-error">{repo.error}</span>}
          </div>
          {repo.commitsSinceTag > 0 && repo.latestTag && (
            <div className="mt-1 text-text-disabled">
              +{repo.commitsSinceTag} since {repo.latestTag}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
