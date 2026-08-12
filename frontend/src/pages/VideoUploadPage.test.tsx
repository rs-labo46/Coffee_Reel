import { act, fireEvent, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ApiClientError } from "../api/client";
import { uploadVideo, VideoUploadError } from "../api/upload";
import { completeVideoUpload, startVideoUpload } from "../api/video";
import { startUploadResponse } from "../tests/videoFixtures";
import VideoUploadPage from "./VideoUploadPage";

const navigateMock = vi.hoisted(() => vi.fn());

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();

  return {
    ...actual,
    useNavigate: () => navigateMock,
  };
});

vi.mock("../api/video", () => ({
  startVideoUpload: vi.fn(),
  completeVideoUpload: vi.fn(),
}));

vi.mock("../api/upload", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../api/upload")>();

  return {
    ...actual,
    uploadVideo: vi.fn(),
  };
});

const startVideoUploadMock = vi.mocked(startVideoUpload);
const completeVideoUploadMock = vi.mocked(completeVideoUpload);
const uploadVideoMock = vi.mocked(uploadVideo);
const originalCreateElement = document.createElement.bind(document);

type Deferred<T> = {
  promise: Promise<T>;
  resolve: (value: T | PromiseLike<T>) => void;
  reject: (reason?: unknown) => void;
};

// 外部から完了時点を制御できるPromiseを生成
function deferred<T>(): Deferred<T> {
  let resolvePromise!: (value: T | PromiseLike<T>) => void;
  let rejectPromise!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });

  return {
    promise,
    resolve: resolvePromise,
    reject: rejectPromise,
  };
}

// Metadata取得用Video Elementへ指定値を設定
function mockVideoMetadata({
  duration = 2,
  width = 720,
  height = 1280,
}: {
  duration?: number;
  width?: number;
  height?: number;
} = {}): void {
  vi.spyOn(document, "createElement").mockImplementation(
    (tagName: string, options?: ElementCreationOptions) => {
      const element = originalCreateElement(tagName, options);

      if (tagName.toLowerCase() !== "video") {
        return element;
      }

      const video = element as HTMLVideoElement;
      let source = "";

      Object.defineProperties(video, {
        duration: {
          configurable: true,
          value: duration,
        },
        videoWidth: {
          configurable: true,
          value: width,
        },
        videoHeight: {
          configurable: true,
          value: height,
        },
        src: {
          configurable: true,
          get: () => source,
          set: (value: string) => {
            source = value;
            queueMicrotask(() => {
              video.onloadedmetadata?.(new Event("loadedmetadata"));
            });
          },
        },
        load: {
          configurable: true,
          value: vi.fn(),
        },
      });

      return video;
    },
  );
}

// 投稿画面をRouter内で描画
function renderPage() {
  return render(
    <MemoryRouter>
      <VideoUploadPage />
    </MemoryRouter>,
  );
}

// 有効な動画Fileを画面へ選択
async function selectValidVideo(): Promise<File> {
  const file = new File([new Uint8Array(1024)], "coffee.mp4", {
    type: "video/mp4",
    lastModified: 1,
  });

  fireEvent.change(screen.getByLabelText("動画ファイル"), {
    target: {
      files: [file],
    },
  });

  expect(await screen.findByText("2.0秒")).toBeInTheDocument();

  return file;
}

describe("VideoUploadPage", () => {
  beforeEach(() => {
    navigateMock.mockReset();
    startVideoUploadMock.mockReset();
    completeVideoUploadMock.mockReset();
    uploadVideoMock.mockReset();

    vi.stubGlobal(
      "URL",
      class TestURL extends URL {
        static createObjectURL = vi.fn(() => "blob:coffee-video");
        static revokeObjectURL = vi.fn();
      },
    );
  });

  it("39,062,300 bytesの動画を投稿開始へ渡せる", async () => {
    const user = userEvent.setup();
    mockVideoMetadata();
    startVideoUploadMock.mockResolvedValue(startUploadResponse());
    uploadVideoMock.mockResolvedValue(undefined);
    completeVideoUploadMock.mockResolvedValue({
      id: 10,
      processing_status: "uploaded",
      publish_status: "private",
    });

    renderPage();
    await user.type(screen.getByLabelText("タイトル"), "圧縮対象テスト");

    const largeFile = new File(["video"], "large.mp4", {
      type: "video/mp4",
      lastModified: 1,
    });
    Object.defineProperty(largeFile, "size", {
      configurable: true,
      value: 39_062_300,
    });

    fireEvent.change(screen.getByLabelText("動画ファイル"), {
      target: {
        files: [largeFile],
      },
    });

    expect(await screen.findByText("2.0秒")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "動画を投稿" }));

    expect(startVideoUploadMock).toHaveBeenCalledWith(
      expect.objectContaining({
        file_size_bytes: 39_062_300,
      }),
      expect.any(String),
    );
  });

  it("50MBを超える動画をAPI送信前に拒否する", async () => {
    renderPage();

    const oversizedFile = new File(["video"], "large.mp4", {
      type: "video/mp4",
    });
    Object.defineProperty(oversizedFile, "size", {
      configurable: true,
      value: 50_000_001,
    });

    fireEvent.change(screen.getByLabelText("動画ファイル"), {
      target: {
        files: [oversizedFile],
      },
    });

    expect(
      await screen.findByText("動画の容量は50MB以下にしてください"),
    ).toBeInTheDocument();
    expect(startVideoUploadMock).not.toHaveBeenCalled();
  });

  it.each([
    {
      metadata: { duration: 10.1, width: 720, height: 1280 },
      message: "動画の再生時間は10秒以下にしてください",
    },
    {
      metadata: { duration: 2, width: 1081, height: 1920 },
      message: "動画の解像度は1080×1920以下にしてください",
    },
    {
      metadata: { duration: 2, width: 720, height: 720 },
      message: "縦横比が9:16の縦動画を選択してください",
    },
  ])("動画Metadata条件違反を表示する", async ({ metadata, message }) => {
    mockVideoMetadata(metadata);
    renderPage();

    fireEvent.change(screen.getByLabelText("動画ファイル"), {
      target: {
        files: [
          new File(["video"], "coffee.mp4", {
            type: "video/mp4",
          }),
        ],
      },
    });

    expect(await screen.findByText(message)).toBeInTheDocument();
    expect(startVideoUploadMock).not.toHaveBeenCalled();
  });

  it("投稿Button連打でも投稿開始とStorage Uploadを1回だけ実行する", async () => {
    const user = userEvent.setup();
    const uploadDeferred = deferred<void>();

    mockVideoMetadata();
    startVideoUploadMock.mockResolvedValue(startUploadResponse());
    uploadVideoMock.mockReturnValue(uploadDeferred.promise);
    completeVideoUploadMock.mockResolvedValue({
      id: 10,
      processing_status: "uploaded",
      publish_status: "private",
    });

    renderPage();
    await user.type(screen.getByLabelText("タイトル"), "抽出テスト");
    await selectValidVideo();

    const submitButton = screen.getByRole("button", { name: "動画を投稿" });
    await user.click(submitButton);
    await user.click(submitButton);

    expect(startVideoUploadMock).toHaveBeenCalledTimes(1);
    expect(uploadVideoMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      uploadDeferred.resolve(undefined);
      await uploadDeferred.promise;
    });
  });

  it("Storage Uploadから受け取った進捗を表示する", async () => {
    const user = userEvent.setup();
    const uploadDeferred = deferred<void>();

    mockVideoMetadata();
    startVideoUploadMock.mockResolvedValue(startUploadResponse());
    uploadVideoMock.mockImplementation((options) => {
      options.onProgress?.({
        loadedBytes: 512,
        totalBytes: 1024,
        percentage: 50,
      });
      return uploadDeferred.promise;
    });
    completeVideoUploadMock.mockResolvedValue({
      id: 10,
      processing_status: "uploaded",
      publish_status: "private",
    });

    renderPage();
    await user.type(screen.getByLabelText("タイトル"), "抽出テスト");
    await selectValidVideo();
    await user.click(screen.getByRole("button", { name: "動画を投稿" }));

    expect(await screen.findByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "50",
    );

    await act(async () => {
      uploadDeferred.resolve(undefined);
      await uploadDeferred.promise;
    });
  });

  it("Storage Upload失敗を利用者向けMessageとして表示する", async () => {
    const user = userEvent.setup();

    mockVideoMetadata();
    startVideoUploadMock.mockResolvedValue(startUploadResponse());
    uploadVideoMock.mockRejectedValue(
      new VideoUploadError(
        "network_error",
        "動画ストレージへ接続できません",
      ),
    );

    renderPage();
    await user.type(screen.getByLabelText("タイトル"), "抽出テスト");
    await selectValidVideo();
    await user.click(screen.getByRole("button", { name: "動画を投稿" }));

    expect(
      await screen.findByText("動画ストレージへ接続できません"),
    ).toBeInTheDocument();
  });

  it("完了通知だけが失敗した再送では動画を再Uploadしない", async () => {
    const user = userEvent.setup();

    mockVideoMetadata();
    startVideoUploadMock.mockResolvedValue(startUploadResponse());
    uploadVideoMock.mockResolvedValue(undefined);
    completeVideoUploadMock
      .mockRejectedValueOnce(
        new ApiClientError(
          0,
          "network_error",
          "APIへ接続できません",
          "request-1",
        ),
      )
      .mockResolvedValueOnce({
        id: 10,
        processing_status: "uploaded",
        publish_status: "private",
      });

    renderPage();
    await user.type(screen.getByLabelText("タイトル"), "抽出テスト");
    await selectValidVideo();
    await user.click(screen.getByRole("button", { name: "動画を投稿" }));

    expect(
      await screen.findByRole("button", { name: "完了通知を再送" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("APIへ接続できません");
    expect(screen.getByRole("alert")).toHaveTextContent("Request ID: request-1");

    await user.click(screen.getByRole("button", { name: "完了通知を再送" }));

    expect(startVideoUploadMock).toHaveBeenCalledTimes(1);
    expect(uploadVideoMock).toHaveBeenCalledTimes(1);
    expect(completeVideoUploadMock).toHaveBeenCalledTimes(2);
    expect(navigateMock).toHaveBeenCalledWith("/me/videos", {
      replace: true,
      state: {
        uploadCompleted: true,
        videoID: 10,
      },
    });
  });
});
