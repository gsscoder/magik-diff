import ReactMarkdown from "react-markdown";
import {checks} from "../wailsjs/go/models";

export function ExpandCollapseIcon({expanded}: {expanded: boolean}) {
    return (
        <svg
            width="14"
            height="14"
            viewBox="0 0 16 16"
            fill="none"
            aria-hidden="true"
            style={{transform: expanded ? "rotate(180deg)" : undefined}}
        >
            <path d="M5 3l4 5-4 5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
            <path d="M9 3l4 5-4 5" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
    );
}

function SparkleIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <path d="M8 1l1.2 3.8L13 6l-3.8 1.2L8 11l-1.2-3.8L3 6l3.8-1.2L8 1z" />
            <path d="M13 10.5l.6 1.9 1.9.6-1.9.6-.6 1.9-.6-1.9-1.9-.6 1.9-.6.6-1.9z" />
        </svg>
    );
}

export type CheckState = { kind: "running" } | { kind: "error"; message: string } | { kind: "done"; result: string };

interface ExplanationPaneProps {
    explanation: string;
    explaining: boolean;
    explainError: string;
    explainedCount: number;
    explanationExpanded: boolean;
    onExplanationExpandedChange: (expanded: boolean) => void;
    onExplain: () => void;
    ready: boolean;
    checkedCount: number;
    checksList: checks.Check[];
    checkResults: Record<string, CheckState>;
    onRunCheck: (check: checks.Check) => void;
}

function ExplanationPane(props: ExplanationPaneProps) {
    const {
        explanation,
        explaining,
        explainError,
        explainedCount,
        explanationExpanded,
        onExplanationExpandedChange,
        onExplain,
        ready,
        checkedCount,
        checksList,
        checkResults,
        onRunCheck,
    } = props;
    return (
        <div className="explanation-pane">
            <div className="explanation-header">
                <button
                    className="explanation-toggle-button"
                    aria-label={explanationExpanded ? "Collapse" : "Expand"}
                    title={explanationExpanded ? "Collapse" : "Expand"}
                    onClick={() => onExplanationExpandedChange(!explanationExpanded)}
                >
                    <ExpandCollapseIcon expanded={explanationExpanded} />
                </button>
                <button
                    className={explaining ? "explain-button explaining" : "explain-button"}
                    aria-label={explaining ? "Explaining…" : "Explain"}
                    title={explaining ? "Explaining…" : "Explain"}
                    disabled={!ready || checkedCount === 0 || explaining}
                    onClick={onExplain}
                >
                    <SparkleIcon />
                </button>
            </div>
            {checksList.length > 0 && (
                <>
                    <div className="explanation-divider" />
                    <div className="check-buttons-row">
                        {checksList.map((check) => {
                            const state = checkResults[check.Name];
                            const running = state?.kind === "running";
                            const disabled = !ready
                                || checkedCount === 0
                                || running;
                            return (
                                <button
                                    key={check.Name}
                                    className="check-button"
                                    title={check.Description}
                                    disabled={disabled}
                                    onClick={() => onRunCheck(check)}
                                >
                                    {running ? `${check.Name}…` : check.Name}
                                </button>
                            );
                        })}
                    </div>
                </>
            )}
            {!explaining && !explanation && !explainError && (
                <p className="placeholder">
                    {checkedCount > 0
                        ? "Click Explain to see an explanation of the checked files"
                        : "Check one or more files, then click Explain"}
                </p>
            )}
            {explaining && (
                <p className="placeholder">Explaining…</p>
            )}
            {explainError && (
                <p className="explain-error">{explainError}</p>
            )}
            {explanation && explainedCount > 1 && (
                <p className="explanation-scope-label">{explainedCount} files</p>
            )}
            {explanation && (
                <div className="markdown-body">
                    <ReactMarkdown>{explanation}</ReactMarkdown>
                </div>
            )}
            {Object.keys(checkResults).length > 0 && (
                <div className="check-results-box">
                    {Object.entries(checkResults).map(([name, state]) => (
                        <div key={name} className="check-result">
                            <p className="check-result-heading"><strong>{name}</strong></p>
                            {state.kind === "running" && (
                                <p className="placeholder">Running…</p>
                            )}
                            {state.kind === "error" && (
                                <p className="explain-error">{state.message}</p>
                            )}
                            {state.kind === "done" && (
                                <div className="markdown-body">
                                    <ReactMarkdown>{state.result}</ReactMarkdown>
                                </div>
                            )}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

export default ExplanationPane;
