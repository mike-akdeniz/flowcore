import dagre from "@dagrejs/dagre";
import { MarkerType, type Edge, type Node } from "@xyflow/react";
import type { Workflow } from "../api";

export const NODE_WIDTH = 210;
export const STEP_HEIGHT = 62;
export const STATUS_HEIGHT = 40;

// A workflow becomes a graph, and the graph is laid out automatically.
//
// FlowCore stores no coordinates — a definition is steps and routing, with no
// idea where anything sits on a page. Decision 14 chose to compute positions
// rather than store them: stored coordinates mean orphan rows when a step is
// deleted and missing positions when one arrives by another path, which is a
// permanent tax on something that is not the point. Recomputing also keeps the
// diagram readable, which is the actual requirement.
export function toGraph(workflow: Workflow): { nodes: Node[]; edges: Edge[] } {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({ rankdir: "TB", ranksep: 64, nodesep: 28 });

  const statusName = new Map(workflow.statuses.map((s) => [s.id, s.name]));

  // Only statuses something actually terminates in are drawn. A status merely
  // shown while a run sits on a step is a label on that step, not a node — a
  // distinction the library makes and the picture should keep.
  const terminal = new Set<string>();
  for (const step of workflow.steps) {
    for (const action of step.actions) {
      if (action.terminalStatusId) terminal.add(action.terminalStatusId);
    }
  }

  for (const step of workflow.steps) {
    graph.setNode(step.id, { width: NODE_WIDTH, height: STEP_HEIGHT });
  }
  for (const id of terminal) {
    graph.setNode(id, { width: NODE_WIDTH, height: STATUS_HEIGHT });
  }

  const edges: Edge[] = [];
  for (const step of workflow.steps) {
    for (const action of step.actions) {
      const target = action.nextStepId ?? action.terminalStatusId;
      if (!target) continue;

      graph.setEdge(step.id, target);
      edges.push({
        id: action.id,
        source: step.id,
        target,
        label: action.name,
        labelBgPadding: [6, 2],
        labelBgBorderRadius: 4,
        labelBgStyle: { fill: "var(--mantine-color-body)", fillOpacity: 0.9 },
        labelStyle: { fill: "var(--mantine-color-text)" },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          color: "var(--mantine-primary-color-filled)",
          width: 16,
          height: 16,
        },
        style: { strokeWidth: 1.5 },
      });
    }
  }

  dagre.layout(graph);

  const nodes: Node[] = workflow.steps.map((step) => {
    const { x, y } = graph.node(step.id);

    return {
      id: step.id,
      type: "step",
      position: { x: x - NODE_WIDTH / 2, y: y - STEP_HEIGHT / 2 },
      data: {
        name: step.name,
        assignee: step.assignee,
        isAgent: step.isAgent,
        isEntry: step.id === workflow.entryStepId,
        isDeadEnd: step.actions.length === 0,
      },
    };
  });

  for (const id of terminal) {
    const { x, y } = graph.node(id);

    nodes.push({
      id,
      type: "terminal",
      position: { x: x - NODE_WIDTH / 2, y: y - STATUS_HEIGHT / 2 },
      data: { name: statusName.get(id) ?? "unknown" },
    });
  }

  return { nodes, edges };
}
