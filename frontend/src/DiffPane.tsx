import React from "react";
import {diffparse} from "../wailsjs/go/models";
import {splitRows} from "./splitRows";

function renderLineContent(line: diffparse.Line) {
    const hl = line.Highlight;
    if (hl.End <= hl.Start) {
        return line.Content;
    }
    const chars = [...line.Content];
    return (
        <>
            {chars.slice(0, hl.Start).join("")}
            <span className="diff-hl">{chars.slice(hl.Start, hl.End).join("")}</span>
            {chars.slice(hl.End).join("")}
        </>
    );
}

interface DiffPaneProps {
    parsedDiff: diffparse.FileDiff | null;
    diffError: string;
    splitView: boolean;
    onSplitViewChange: (v: boolean) => void;
}

function DiffPane(props: DiffPaneProps) {
    const {parsedDiff, diffError, splitView, onSplitViewChange} = props;
    return (
        <div className="diff-pane">
            {!parsedDiff && !diffError && (
                <p className="placeholder">Select a file to see its diff</p>
            )}
            {diffError && (
                <p className="explain-error">{diffError}</p>
            )}
            {parsedDiff && parsedDiff.Path !== "" && (
                <div className="diff-file-header">
                    <span className="diff-file-path">{parsedDiff.Path}</span>
                    {(parsedDiff.Hunks ?? []).length > 0 && (
                        <div className="view-toggle">
                            <button
                                className={!splitView ? "active" : ""}
                                onClick={() => onSplitViewChange(false)}
                            >
                                Unified
                            </button>
                            <button
                                className={splitView ? "active" : ""}
                                onClick={() => onSplitViewChange(true)}
                            >
                                Split
                            </button>
                        </div>
                    )}
                </div>
            )}
            {parsedDiff && (parsedDiff.Hunks ?? []).length === 0 && (
                <p className="placeholder">No textual diff to display (untracked, binary, or rename-only)</p>
            )}
            {!splitView && parsedDiff && (parsedDiff.Hunks ?? []).map((hunk, hi) => (
                <div key={hi} className="diff-hunk">
                    <div className="diff-hunk-header">{hunk.Header}</div>
                    {(hunk.Lines ?? []).map((line, li) => (
                        <div key={li} className={`diff-line ${line.Type}`}>
                            <span className="diff-gutter">{line.OldNum > 0 ? line.OldNum : ""}</span>
                            <span className="diff-gutter">{line.NewNum > 0 ? line.NewNum : ""}</span>
                            <span className="diff-sign">
                                {line.Type === "added" ? "+" : line.Type === "removed" ? "-" : " "}
                            </span>
                            <span className="diff-content">{renderLineContent(line)}</span>
                        </div>
                    ))}
                </div>
            ))}
            {splitView && parsedDiff && (parsedDiff.Hunks ?? []).map((hunk, hi) => (
                <div key={hi} className="diff-hunk">
                    <div className="diff-hunk-header">{hunk.Header}</div>
                    {splitRows(hunk.Lines ?? []).map((row, ri) => (
                        <div key={ri} className="split-row">
                            <div
                                className={`split-cell split-cell-left${
                                    row.left
                                        ? row.left.Type === "removed" ? " removed" : ""
                                        : " split-filler"
                                }`}
                            >
                                {row.left && (
                                    <>
                                        <span className="diff-gutter">{row.left.OldNum > 0 ? row.left.OldNum : ""}</span>
                                        <span className="diff-content">{renderLineContent(row.left)}</span>
                                    </>
                                )}
                            </div>
                            <div
                                className={`split-cell split-cell-right${
                                    row.right
                                        ? row.right.Type === "added" ? " added" : ""
                                        : " split-filler"
                                }`}
                            >
                                {row.right && (
                                    <>
                                        <span className="diff-gutter">{row.right.NewNum > 0 ? row.right.NewNum : ""}</span>
                                        <span className="diff-content">{renderLineContent(row.right)}</span>
                                    </>
                                )}
                            </div>
                        </div>
                    ))}
                </div>
            ))}
        </div>
    );
}

export default React.memo(DiffPane);
