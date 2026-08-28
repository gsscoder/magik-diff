import {useEffect, useRef, useState} from "react";
import {Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise} from "../wailsjs/runtime/runtime";
import {WorkingDir} from "../wailsjs/go/main/App";

function FolderIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
            <path d="M1 4a1 1 0 0 1 1-1h3.5l1.5 1.5H14a1 1 0 0 1 1 1V12a1 1 0 0 1-1 1H2a1 1 0 0 1-1-1V4z" />
        </svg>
    );
}

function MinimiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1 5H9" stroke="currentColor" />
        </svg>
    );
}

function MaximiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <rect x="1.5" y="1.5" width="7" height="7" stroke="currentColor" />
        </svg>
    );
}

function RestoreIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M3 1H9V7" stroke="currentColor" />
            <rect x="1" y="3" width="6" height="6" stroke="currentColor" />
        </svg>
    );
}

function CloseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1.5 1.5L8.5 8.5M8.5 1.5L1.5 8.5" stroke="currentColor" />
        </svg>
    );
}

function TitleBar() {
    const [cwd, setCwd] = useState("");
    const [maximised, setMaximised] = useState(false);
    const resizeFrame = useRef(0);

    useEffect(() => {
        WorkingDir().then(setCwd);
    }, []);

    useEffect(() => {
        function poll() {
            WindowIsMaximised().then(setMaximised);
        }
        poll();
        // Wails v2.13 emits no window-state-changed event, so Aero-snap and
        // Win+Up-arrow maximise/restore are only observable via the resize
        // event they trigger; coalesce with rAF since resize fires rapidly
        // during a live drag-resize.
        function onResize() {
            cancelAnimationFrame(resizeFrame.current);
            resizeFrame.current = requestAnimationFrame(poll);
        }
        window.addEventListener("resize", onResize);
        return () => {
            window.removeEventListener("resize", onResize);
            cancelAnimationFrame(resizeFrame.current);
        };
    }, []);

    return (
        <div className="titlebar">
            <div className="titlebar-drag" onDoubleClick={() => WindowToggleMaximise()}>
                <FolderIcon />
                <span className="titlebar-cwd">{cwd}</span>
                <span className="titlebar-spacer" />
            </div>
            <div className="titlebar-controls">
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label="Minimise"
                    title="Minimise"
                    onClick={() => WindowMinimise()}
                >
                    <MinimiseIcon />
                </button>
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label={maximised ? "Restore" : "Maximise"}
                    title={maximised ? "Restore" : "Maximise"}
                    onClick={() => WindowToggleMaximise()}
                >
                    {maximised ? <RestoreIcon /> : <MaximiseIcon />}
                </button>
                <button
                    type="button"
                    className="titlebar-button titlebar-close"
                    aria-label="Close"
                    title="Close"
                    onClick={() => Quit()}
                >
                    <CloseIcon />
                </button>
            </div>
        </div>
    );
}

export default TitleBar;
