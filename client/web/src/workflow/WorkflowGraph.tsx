import { useMemo } from "react";
import { Background, Controls, ReactFlow } from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import type { Workflow } from "../api";
import { toGraph } from "./layout";
import { StepNode, TerminalNode } from "./nodes";

const nodeTypes = { step: StepNode, terminal: TerminalNode };

// One graph component, used read-only here and with editing switched on later.
//
// That is why decision 14 chose automatic layout: with no stored coordinates the
// read view and the editor are the same picture, drawn once.
export function WorkflowGraph({
  workflow,
  height = 560,
  onSelectStep,
  selectedStepId,
}: {
  workflow: Workflow;
  height?: number;
  onSelectStep?: (stepId: string) => void;
  selectedStepId?: string;
}) {
  const { nodes, edges } = useMemo(() => toGraph(workflow), [workflow]);

  const marked = useMemo(
    () =>
      nodes.map((node) => ({
        ...node,
        selected: node.id === selectedStepId,
      })),
    [nodes, selectedStepId],
  );

  return (
    <div style={{ height }}>
      <ReactFlow
        nodes={marked}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        nodesDraggable={false}
        nodesConnectable={false}
        edgesFocusable={false}
        proOptions={{ hideAttribution: false }}
        onNodeClick={(_, node) => onSelectStep?.(node.id)}
      >
        <Background gap={16} size={1} />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
