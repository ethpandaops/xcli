import type { RepoInfo } from '@/types';
import Spinner from '@/components/Spinner';

interface GitStatusProps {
  repos: RepoInfo[];
}

export default function GitStatus({ repos }: GitStatusProps) {
  if (repos.length === 0) {
    return (
      <div className="rounded-sm border border-border bg-surface-light p-4">
        <h3 className="mb-2 text-sm/5 font-semibold text-gray-400">Git Status</h3>
        <Spinner />
      </div>
    );
  }

  return (
    <div className="rounded-sm border border-border bg-surface-light p-4">
      <h3 className="mb-3 text-sm/5 font-semibold text-gray-400">Git Status</h3>
      <div className="flex flex-col gap-2">
        {repos.map(repo => (
          <div key={repo.name} className="rounded-xs bg-surface px-3 py-2 text-xs/4">
            <div className="flex items-center justify-between">
              <span className="font-medium text-gray-200">{repo.name}</span>
              <span className="font-mono text-gray-500">{repo.branch}</span>
            </div>
            <div className="mt-1 flex gap-2">
              {repo.behindBy > 0 && <span className="text-amber-400">{repo.behindBy} behind</span>}
              {repo.aheadBy > 0 && <span className="text-sky-400">{repo.aheadBy} ahead</span>}
              {repo.hasUncommitted && <span className="text-orange-400">{repo.uncommittedCount} uncommitted</span>}
              {repo.isUpToDate && !repo.hasUncommitted && !repo.error && (
                <span className="text-emerald-500">up to date</span>
              )}
              {repo.error && <span className="text-red-400">{repo.error}</span>}
            </div>
            {repo.commitsSinceTag > 0 && repo.latestTag && (
              <div className="mt-1 text-gray-600">
                +{repo.commitsSinceTag} since {repo.latestTag}
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
