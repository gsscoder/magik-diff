import {useEffect, useState} from "react";
import {CurrentBranch, WorkingDir} from "../wailsjs/go/main/App";
import {EventsOn} from "../wailsjs/runtime/runtime";

function FolderIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <path d="M1 4a1 1 0 0 1 1-1h3.5l1.5 1.5H14a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1H2a1 1 0 0 1-1-1V4z" />
        </svg>
    );
}

function BranchIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <path d="M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.492 2.492 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Zm-6 0a.75.75 0 1 0 1.5 0 .75.75 0 0 0-1.5 0Zm8.25-.75a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5ZM4.25 12a.75.75 0 1 0 0 1.5.75.75 0 0 0 0-1.5Z" />
        </svg>
    );
}

function StatusBar() {
    const [cwd, setCwd] = useState("");
    const [branch, setBranch] = useState("");

    useEffect(() => {
        WorkingDir().then(setCwd).catch(() => {});
        CurrentBranch().then(setBranch).catch(() => {});
    }, []);

    // Reflects a branch switch (or repo mutation) made by an external
    // process while the app is open.
    useEffect(() => {
        return EventsOn("repo:changed", () => {
            WorkingDir().then(setCwd).catch(() => {});
            CurrentBranch().then(setBranch).catch(() => {});
        });
    }, []);

    return (
        <div className="statusbar">
            <div className="statusbar-segment" title={cwd}>
                <FolderIcon />
                <span className="statusbar-cwd">{cwd}</span>
            </div>
            {branch !== "" && (
                <>
                    <span className="statusbar-divider" />
                    <div className="statusbar-segment" title={branch}>
                        <BranchIcon />
                        <span className="statusbar-branch">{branch}</span>
                    </div>
                </>
            )}
        </div>
    );
}

export default StatusBar;
