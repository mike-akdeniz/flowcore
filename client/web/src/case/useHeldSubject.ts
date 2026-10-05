import { useCallback, useEffect, useRef, useState } from "react";
import type { AiStepStatus, Case } from "../api";

// How long an AI step's spinner stays on screen once it has been shown.
//
// A result that arrives in half a second is indistinguishable, on screen, from
// nothing having happened: the panel shows "choose a model" and then, with no
// interval, the finished step. Nielsen's first heuristic is visibility of system
// status, and the usual practice is a floor under any busy indicator so that a
// fast operation is still seen to have happened. Slower operations are not
// stretched: the floor applies to the wait, and a model that takes ten seconds
// shows ten.
const minimumWorkingMilliseconds = 1000;

// Queued, being asked, or about to be asked again: states that end on their own.
export function aiStepIsWorking(aiStep: AiStepStatus) {
  return aiStep.state === "queued" || aiStep.state === "running" || aiStep.state === "retrying";
}

type Working = { reference: string; visitId: string; since: number };

// The case as the screen shows it, which can trail the server by up to the
// floor above.
//
// `receive` takes every update the page gets — a load, a poll, the response to
// an action. While the screen is showing an AI step at work, an update that
// says it is over waits until the spinner has been up for the minimum. Anything
// else is shown at once. `expectWork` is for the moment a person does something
// that puts a waiting step to work — choosing a model — when the server has
// already started and the screen has not yet been told: it shows the step as
// running from that moment, which is what the server is in the middle of doing.
export function useHeldSubject() {
  const [subject, setSubject] = useState<Case | null>(null);
  const shown = useRef<Case | null>(null);
  const working = useRef<Working | null>(null);
  const pending = useRef<Case | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout>>();

  const show = useCallback((next: Case) => {
    clearTimeout(timer.current);
    timer.current = undefined;
    pending.current = null;

    const step = next.currentStep;
    if (step?.aiStep && aiStepIsWorking(step.aiStep)) {
      if (working.current?.visitId !== step.visitId) {
        working.current = { reference: next.reference, visitId: step.visitId, since: Date.now() };
      }
    } else {
      working.current = null;
    }

    shown.current = next;
    setSubject(next);
  }, []);

  const receive = useCallback(
    (next: Case) => {
      const held = working.current;
      const step = next.currentStep;
      const stillWorking =
        step?.aiStep && aiStepIsWorking(step.aiStep) && step.visitId === held?.visitId;

      if (!held || held.reference !== next.reference || stillWorking) {
        show(next);

        return;
      }

      const remaining = held.since + minimumWorkingMilliseconds - Date.now();
      if (remaining <= 0) {
        show(next);

        return;
      }

      // Latest wins: a newer update replaces one already waiting.
      pending.current = next;
      if (timer.current === undefined) {
        timer.current = setTimeout(() => {
          if (pending.current) show(pending.current);
        }, remaining);
      }
    },
    [show],
  );

  const expectWork = useCallback((detail: string) => {
    const current = shown.current;
    const step = current?.currentStep;
    if (!current || !step?.aiStep || aiStepIsWorking(step.aiStep)) return;

    working.current = { reference: current.reference, visitId: step.visitId, since: Date.now() };

    const running = { ...current, currentStep: { ...step, aiStep: { state: "running" as const, detail } } };
    shown.current = running;
    setSubject(running);
  }, []);

  useEffect(() => () => clearTimeout(timer.current), []);

  return { subject, receive, expectWork };
}
