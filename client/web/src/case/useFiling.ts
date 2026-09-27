import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { api, type NewSubmission } from "../api";

// The one thing the two filing forms share: post it, survive a failure, and go
// to the case that came back.
//
// Behaviour, not layout. The forms have no fields in common — a claim and a
// policy application share nothing but a reference — so a single component with
// conditional fields would be two forms wearing one coat. What they genuinely
// share is what happens after Submit, and that lives here.
export function useFiling() {
  const navigate = useNavigate();
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<string>();

  async function file(body: NewSubmission) {
    setBusy(true);
    setFailure(undefined);

    try {
      const { reference } = await api.createCase(body);
      navigate(`/cases/${reference}`);
    } catch (error) {
      setFailure(error instanceof Error ? error.message : "could not file it");
      setBusy(false);
    }
  }

  return { file, busy, failure };
}
