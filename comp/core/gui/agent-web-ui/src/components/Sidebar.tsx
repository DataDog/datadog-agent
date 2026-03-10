import { NavLink } from "react-router";

function StatusIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <circle cx="8" cy="8" r="6.5" />
      <path d="M8 4.5v4M8 10.5v.5" strokeLinecap="round" />
    </svg>
  );
}

function LogIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <rect x="2" y="1.5" width="12" height="13" rx="1.5" />
      <path d="M5 5h6M5 8h6M5 11h3" strokeLinecap="round" />
    </svg>
  );
}

function SettingsIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <circle cx="8" cy="8" r="2" />
      <path d="M8 1.5v2M8 12.5v2M1.5 8h2M12.5 8h2M3.17 3.17l1.42 1.42M11.41 11.41l1.42 1.42M3.17 12.83l1.42-1.42M11.41 4.59l1.42-1.42" strokeLinecap="round" />
    </svg>
  );
}

function CheckIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M3 8.5l3 3 7-7" strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  );
}

function FlareIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M3 2l1 10 4 2 4-2 1-10" strokeLinejoin="round" />
      <path d="M3.5 6h9" />
    </svg>
  );
}

function RestartIcon() {
  return (
    <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
      <path d="M2.5 2.5v4h4" strokeLinecap="round" strokeLinejoin="round" />
      <path d="M3 9a5.5 5.5 0 1 0 1.35-3.6L2.5 6.5" strokeLinecap="round" />
    </svg>
  );
}

const linkClass = ({ isActive }: { isActive: boolean }) =>
  `sidebar-link ${isActive ? "active" : ""}`;

const subLinkClass = ({ isActive }: { isActive: boolean }) =>
  `sidebar-sub-link ${isActive ? "active" : ""}`;

export function Sidebar() {
  return (
    <nav className="sidebar">
      <div className="sidebar-logo">
        <svg viewBox="0 0 216 216" fill="white" xmlns="http://www.w3.org/2000/svg">
          <path d="M177.9,88.5l-44.8,8.1c-1.1,1.4-3.9,3.9-5.2,4.6c-5.7,2.8-9.5,2-12.8,1.2c-2.1-0.5-3.4-0.8-5.1-1.6 l-10.9,1.5l6.6,55.4l76.7-13.8L177.9,88.5z M109.7,152.3l-0.6-6l12.5-19.1l14.2,4.2l12.2-20.4l14.7,9.7l11.1-23.4l4,42.6 L109.7,152.3z" />
        </svg>
      </div>

      <ul className="sidebar-nav">
        <li className="sidebar-item">
          <span className="sidebar-section-title">
            <StatusIcon /> Status
          </span>
          <NavLink to="/status/general" className={subLinkClass}>
            General
          </NavLink>
          <NavLink to="/status/collector" className={subLinkClass}>
            Collector
          </NavLink>
        </li>

        <li className="sidebar-item">
          <NavLink to="/log" className={linkClass}>
            <LogIcon /> Log
          </NavLink>
        </li>

        <li className="sidebar-item">
          <NavLink to="/settings" className={linkClass}>
            <SettingsIcon /> Settings
          </NavLink>
        </li>

        <li className="sidebar-item">
          <span className="sidebar-section-title">
            <CheckIcon /> Checks
          </span>
          <NavLink to="/checks/manage" className={subLinkClass}>
            Manage Checks
          </NavLink>
          <NavLink to="/checks/running" className={subLinkClass}>
            Checks Summary
          </NavLink>
        </li>

        <li className="sidebar-item">
          <NavLink to="/flare" className={linkClass}>
            <FlareIcon /> Flare
          </NavLink>
        </li>

        <li className="sidebar-item">
          <NavLink to="/restart" className={linkClass}>
            <RestartIcon /> Restart Agent
          </NavLink>
        </li>
      </ul>
    </nav>
  );
}
