import { useEffect, useState } from "react";
import { Navigate, Route, Routes } from "react-router-dom";
import { Center, Loader } from "@mantine/core";
import { api, type Models, type Session } from "./api";
import { SignIn } from "./pages/SignIn";
import { CaseView } from "./pages/CaseView";
import { AllWork } from "./pages/AllWork";
import { MyWork } from "./pages/MyWork";
import { NewCase } from "./pages/NewCase";
import { Workflows } from "./pages/Workflows";
import { WorkflowEdit } from "./pages/WorkflowEdit";
import { WorkflowView } from "./pages/WorkflowView";
import { Shell } from "./Shell";

export function App() {
  const [session, setSession] = useState<Session | null>(null);
  // The session's model, held here because two places read it: the picker in
  // the top bar, and the case screen's note on who will read a new document.
  const [models, setModels] = useState<Models | null>(null);

  const reload = () => api.session().then(setSession);

  useEffect(() => {
    void reload();
  }, []);

  const signedIn = session?.signedInAs?.reference;

  useEffect(() => {
    if (signedIn) void api.models().then(setModels);
  }, [signedIn]);

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
    <Shell session={session} models={models} onModelsChanged={setModels} onSignedOut={reload}>
      {/* Keyed on who is signed in, so switching identity remounts the screen
          and it refetches. Every page here is "what does this person see", so
          re-reading on a switch is the rule rather than an exception — making it
          structural beats remembering to add a dependency to each new page. */}
      <Routes key={session.signedInAs.reference}>
        <Route path="/" element={<MyWork />} />
        <Route path="/cases" element={<AllWork />} />
        <Route path="/cases/new" element={<NewCase />} />
        <Route
          path="/cases/:reference"
          element={
            <CaseView
              model={models?.chosen && models.available ? models.chosen.label : null}
              identity={session.signedInAs}
            />
          }
        />
        <Route path="/workflows" element={<Workflows />} />
        <Route path="/workflows/:id" element={<WorkflowView />} />
        <Route path="/workflows/:id/edit" element={<WorkflowEdit />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}
