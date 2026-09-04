import ReactMarkdown from "react-markdown";
import {brief} from "../wailsjs/go/models";
import {ExpandCollapseIcon} from "./ExplanationPane";

function RefreshIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="M13.5 8a5.5 5.5 0 1 1-1.6-3.9" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" />
            <path d="M13.5 2.5v3.5h-3.5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
    );
}

interface ProjectBriefBarProps {
    state: brief.State | null;
    acquiring: boolean;
    error: string;
    onAcquire: () => void;
    expanded: boolean;
    onExpandedChange: (expanded: boolean) => void;
    useBrief: boolean;
    onUseBriefChange: (use: boolean) => void;
}

function ProjectBriefBar(props: ProjectBriefBarProps) {
    const {state, acquiring, error, onAcquire, expanded, onExpandedChange, useBrief, onUseBriefChange} = props;

    if (!state || !state.HasSources) {
        return null;
    }

    if (!state.Stored) {
        return (
            <div className="project-brief-bar">
                <button
                    className={acquiring ? "project-brief-acquire acquiring" : "project-brief-acquire"}
                    disabled={acquiring}
                    onClick={onAcquire}
                >
                    <RefreshIcon />
                    {acquiring ? "Acquiring project brief…" : "Acquire project brief"}
                </button>
                {error && <p className="explain-error project-brief-error">{error}</p>}
            </div>
        );
    }

    return (
        <div className="project-brief-bar">
            <button
                className="project-brief-header"
                aria-label={expanded ? "Collapse project brief" : "Expand project brief"}
                onClick={() => onExpandedChange(!expanded)}
            >
                <ExpandCollapseIcon expanded={expanded} />
                <span>Project brief</span>
                {state.Stale && (
                    <span className="project-brief-stale-dot" title="Source files changed since this brief was extracted" />
                )}
            </button>
            {expanded && (
                <div className="project-brief-body">
                    <div className="markdown-body">
                        <ReactMarkdown>{state.Brief.text}</ReactMarkdown>
                    </div>
                    <div className="project-brief-actions">
                        <label className="project-brief-use">
                            <input
                                type="checkbox"
                                checked={useBrief}
                                onChange={(e) => onUseBriefChange(e.target.checked)}
                            />
                            Use for Explain
                        </label>
                        <button
                            className={[
                                "project-brief-refresh",
                                acquiring && "acquiring",
                                state.Stale && "stale",
                            ].filter(Boolean).join(" ")}
                            aria-label="Refresh project brief"
                            title={state.Stale ? "Source files changed — refresh the brief" : "Refresh project brief"}
                            disabled={acquiring}
                            onClick={onAcquire}
                        >
                            <RefreshIcon />
                        </button>
                    </div>
                    {error && <p className="explain-error project-brief-error">{error}</p>}
                </div>
            )}
        </div>
    );
}

export default ProjectBriefBar;
