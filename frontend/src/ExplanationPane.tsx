import ReactMarkdown from "react-markdown";
import {checks} from "../wailsjs/go/models";

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
                    onClick={() => onExplanationExpandedChange(!explanationExpanded)}
                >
                    {explanationExpanded ? "⇤ Collapse" : "⇥ Expand"}
                </button>
                <button
                    className="explain-button"
                    disabled={!ready || checkedCount === 0 || explaining}
                    onClick={onExplain}
                >
                    {explaining ? "Explaining…" : "Explain"}
                </button>
            </div>
            {checksList.length > 0 && (
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
