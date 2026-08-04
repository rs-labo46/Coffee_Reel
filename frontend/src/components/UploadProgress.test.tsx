import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import UploadProgress from "./UploadProgress";

describe("UploadProgress", () => {
  it("Upload進捗率と転送済み容量を表示する", () => {
    render(
      <UploadProgress
        fileName="coffee.mp4"
        progress={{
          loadedBytes: 512 * 1024,
          totalBytes: 1024 * 1024,
          percentage: 50,
        }}
      />,
    );

    expect(screen.getByText("coffee.mp4")).toBeInTheDocument();
    expect(screen.getByText("50%")).toBeInTheDocument();
    expect(screen.getByLabelText("アップロード済みの容量")).toHaveTextContent(
      "512.0 KB / 1.0 MB",
    );
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "50",
    );
  });

  it("進捗率を0〜100へ補正する", () => {
    render(
      <UploadProgress
        fileName="coffee.mp4"
        progress={{
          loadedBytes: 200,
          totalBytes: 100,
          percentage: 130,
        }}
      />,
    );

    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "100",
    );
    expect(screen.getByText("アップロード完了")).toBeInTheDocument();
  });

  it("Upload中断ボタンから親処理を呼ぶ", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn();

    render(
      <UploadProgress
        fileName="coffee.mp4"
        progress={{
          loadedBytes: 1,
          totalBytes: 10,
          percentage: 10,
        }}
        onCancel={onCancel}
      />,
    );

    await user.click(
      screen.getByRole("button", { name: "アップロードを中断" }),
    );

    expect(onCancel).toHaveBeenCalledTimes(1);
  });
});
