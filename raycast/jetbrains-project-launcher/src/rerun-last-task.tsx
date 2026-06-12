import { RerunTask } from "./lib/backend";

// The last-run task is a single global record, so re-running it is only useful
// when you don't first have to find a project — hence its own top-level command
// rather than an action buried under a project row.
export default function RerunLastTask() {
  return <RerunTask />;
}
