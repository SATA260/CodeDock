import { ChatRoute } from "../../chat-route";

export default async function SessionPage({
  params,
}: {
  params: Promise<{ sessionId: string }>;
}) {
  const { sessionId } = await params;
  return <ChatRoute sessionId={sessionId} />;
}
