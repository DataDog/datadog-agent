import { BrowserRouter, Routes, Route, Navigate } from "react-router";
import { Layout } from "./components/Layout";
import { StatusPage } from "./pages/StatusPage";
import { LogPage } from "./pages/LogPage";
import { SettingsPage } from "./pages/SettingsPage";
import { ManageChecksPage } from "./pages/ManageChecksPage";
import { RunningChecksPage } from "./pages/RunningChecksPage";
import { FlarePage } from "./pages/FlarePage";
import { RestartPage } from "./pages/RestartPage";

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route index element={<Navigate to="/status/general" replace />} />
          <Route path="status/:type" element={<StatusPage />} />
          <Route path="log" element={<LogPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="checks/manage" element={<ManageChecksPage />} />
          <Route path="checks/running" element={<RunningChecksPage />} />
          <Route path="flare" element={<FlarePage />} />
          <Route path="restart" element={<RestartPage />} />
          <Route path="*" element={<Navigate to="/status/general" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
