/** 把 Run 终态收成一行英文说明。 */
export function terminalStatusCopy(input: {
  status: string;
  stopReason?: string;
  error?: string;
}): string {
  if (input.status === "completed") {
    return "Run completed";
  }
  if (input.status === "cancelled") {
    return "Cancelled";
  }
  const error = input.error?.trim();
  if (input.stopReason === "model_error" || input.status === "failed") {
    return error ? `Model request failed: ${error}` : "Model request failed";
  }
  const reason = input.stopReason || input.status;
  return error ? `Run ended: ${reason}. ${error}` : `Run ended: ${reason}`;
}
