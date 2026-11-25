import ReactMarkdown from "react-markdown";
import {brief} from "../wailsjs/go/models";
import {CheckIcon, CopyIcon, ExpandCollapseIcon} from "./ExplanationPane";
import {useCopyToClipboard} from "./useCopyToClipboard";

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
    onOpenDialog: () => void;
    expanded: boolean;
    onExpandedChange: (expanded: boolean) => void;
    useBrief: boolean;
    onUseBriefChange: (use: boolean) => void;
}

function ProjectBriefBar(props: ProjectBriefBarProps) {
    const {state, onOpenDialog, expanded, onExpandedChange, useBrief, onUseBriefChange} = props;
    const {copied, copy: copyBrief} = useCopyToClipboard(state?.Brief.text);

    if (!state) {
        return null;
    }

    if (!state.Stored) {
        return (
            <div className="project-brief-bar">
                <button className="project-brief-acquire" onClick={onOpenDialog}>
                    <RefreshIcon />
                    Acquire project brief
                </button>
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
                        <div className="project-brief-action-buttons">
                            <button
                                className="project-brief-refresh"
                                aria-label="Refresh project brief"
                                title="Refresh project brief"
                                onClick={onOpenDialog}
                            >
                                <RefreshIcon />
                            </button>
                            <button
                                className="project-brief-copy"
                                aria-label={copied ? "Copied" : "Copy project brief"}
                                title={copied ? "Copied" : "Copy project brief"}
                                disabled={!state.Brief.text}
                                onClick={copyBrief}
                            >
                                {copied ? <CheckIcon /> : <CopyIcon />}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

export default ProjectBriefBar;
