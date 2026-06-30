from app.streaming.events import StreamEvent, final_event, token_event


def test_stream_event_renders_sse() -> None:
    event = StreamEvent(event="status", data={"status": "thinking"})

    assert event.to_sse() == 'event: status\ndata: {"status": "thinking"}\n\n'


def test_token_event_uses_token_event_name() -> None:
    event = token_event("hello")

    assert event.event == "token"
    assert event.data == {"content": "hello"}


def test_final_event_marks_done() -> None:
    event = final_event()

    assert event.event == "final"
    assert event.data == {"done": True}
