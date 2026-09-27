import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Center, Loader } from "@mantine/core";
import { api, type Session } from "./api";
import { SignIn } from "./pages/SignIn";
import { CaseView } from "./pages/CaseView";
import { MyWork } from "./pages/MyWork";
import { NewCase } from "./pages/NewCase";
import { Workflows } from "./pages/Workflows";
import { WorkflowView } from "./pages/WorkflowView";
import { Shell } from "./Shell";

export function App() {
  const [session, setSession] = useState<Session | null>(null);

  const reload = () => api.session().then(setSession);

  useEffect(() => {
    void reload();
  }, []);

  if (!session) {
    return (
      <Center h="100vh">
        <Loader />
      </Center>
    );
  }

  if (!session.signedInAs) {
    return <SignIn session={session} onSignedIn={reload} />;
  }

  return (
    <Shell session={session} onSignedOut={reload}>
      {/* Keyed on who is signed in, so switching identity remounts the screen
          and it refetches. Every page here is "what does this person see", so
          re-reading on a switch is the rule rather than an exception — making it
          structural beats remembering to add a dependency to each new page. */}
      <Routes key={session.signedInAs.reference}>
        <Route
          path="/"
          element={<MyWork groups={session.signedInAs.groups} />}
        />
        <Route path="/cases/new" element={<NewCase />} />
        <Route
          path="/cases/:reference"
          element={<CaseView agentMode={session.agentMode} />}
        />
        <Route path="/workflows" element={<Workflows />} />
        <Route path="/workflows/:id" element={<WorkflowView />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}
