import { useState } from "react";
import { AppModule, CharacterLibraryItem, ToolchainModule } from "../../types";
import AssetLibrary from "./AssetLibrary";
import HomeStudio from "./HomeStudio";
import ImageStudio from "./ImageStudio";
import TaskCenter from "./TaskCenter";
import { ToolchainProvider } from "./ToolchainContext";
import UIStudio from "./UIStudio";
import VideoStudio from "./VideoStudio";

interface Props {
  activeModule: AppModule;
  showAssets: boolean;
  showTasks: boolean;
  characters: CharacterLibraryItem[];
  onModuleChange: (module: AppModule) => void;
  onShowAssets: (open: boolean) => void;
  onShowTasks: (open: boolean) => void;
  onNotify: (kind: "info" | "error" | "success", message: string) => void;
  defaults?: {
    imageProvider?: string;
    imageModel?: string;
    videoProvider?: string;
    videoModel?: string;
  };
}

export default function ToolchainHub({
  activeModule,
  showAssets,
  showTasks,
  characters,
  onModuleChange,
  onShowAssets,
  onShowTasks,
  onNotify,
  defaults,
}: Props) {
  const [focusAssetId, setFocusAssetId] = useState<string | undefined>();
  const navigate = (module: ToolchainModule) => {
    onModuleChange(module);
    onShowAssets(false);
  };
  const openAssets = (assetId?: string) => {
    setFocusAssetId(assetId);
    onShowAssets(true);
  };

  return (
    <ToolchainProvider onNotify={onNotify}>
      <div className="toolchain-host" hidden={activeModule === "animation"}>
        <div className="toolchain-module-view module-view-home" hidden={activeModule !== "home"}><HomeStudio active={activeModule === "home"} onNavigate={navigate} onOpenAssets={() => onShowAssets(true)} onOpenTasks={() => onShowTasks(true)} /></div>
        <div className="toolchain-module-view module-view-image" hidden={activeModule !== "image"}><ImageStudio defaultProvider={defaults?.imageProvider} defaultModel={defaults?.imageModel} onOpenAssets={openAssets} onOpenTasks={() => onShowTasks(true)} onNavigate={navigate} /></div>
        <div className="toolchain-module-view module-view-video" hidden={activeModule !== "video"}><VideoStudio defaultProvider={defaults?.videoProvider} defaultModel={defaults?.videoModel} onOpenAssets={openAssets} onOpenTasks={() => onShowTasks(true)} onNavigate={navigate} /></div>
        <div className="toolchain-module-view module-view-ui" hidden={activeModule !== "ui"}><UIStudio defaultProvider={defaults?.imageProvider} defaultModel={defaults?.imageModel} onOpenAssets={openAssets} onOpenTasks={() => onShowTasks(true)} onNavigate={navigate} /></div>
      </div>
      <TaskCenter open={showTasks} onOpenChange={onShowTasks} />
      <AssetLibrary open={showAssets} onOpenChange={onShowAssets} characters={characters} activeModule={activeModule} focusAssetId={focusAssetId} onNavigate={navigate} />
    </ToolchainProvider>
  );
}
