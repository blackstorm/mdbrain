import { type App, Notice, PluginSettingTab, Setting } from "obsidian";
import type MdbrainPlugin from "../main";

export class MdbrainSettingTab extends PluginSettingTab {
  plugin: MdbrainPlugin;

  constructor(app: App, plugin: MdbrainPlugin) {
    super(app, plugin);
    this.plugin = plugin;
  }

  display(): void {
    const { containerEl } = this;

    containerEl.empty();
    containerEl.createEl("h2", { text: "Mdbrain Settings" });

    new Setting(containerEl)
      .setName("Publish URL")
      .setDesc("Your Mdbrain publish URL")
      .addText((text) =>
        text
          .setPlaceholder("https://console.example.com")
          .setValue(this.plugin.settings.serverUrl ?? "")
          .onChange(async (value) => {
            this.plugin.settings.serverUrl = value;
            await this.plugin.saveSettings();
          }),
      );

    new Setting(containerEl)
      .setName("Publish Key")
      .setDesc("Publish Key from Mdbrain Console")
      .addText((text) =>
        text
          .setPlaceholder("xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
          .onChange(async (value) => {
            this.plugin.settings.publishKey = value;
            await this.plugin.saveSettings();
          }),
      );

    new Setting(containerEl)
      .setName("Test publish connection")
      .setDesc("Test connectivity to your Publish URL (30s timeout)")
      .addButton((button) =>
        button.setButtonText("Test").onClick(async () => {
          button.setDisabled(true);
          button.setButtonText("Testing...");

          new Notice("Testing publish connection...");

          const result = await this.plugin.syncClient.getVaultInfo();

          button.setDisabled(false);
          button.setButtonText("Test");

          if (result.success && result.vault) {
            new Notice(
              `Connection successful.\n` +
                `Vault: ${result.vault.name}\n` +
                `Domain: ${result.vault.domain || "Not set"}`,
              5000,
            );
          } else {
            new Notice(
              `Connection failed: ${result.error || "Unknown error"}`,
              5000,
            );
          }
        }),
      );

    new Setting(containerEl)
      .setName("Full publish")
      .setDesc("Manually publish all Markdown files to the server")
      .addButton((button) =>
        button.setButtonText("Start publish").onClick(async () => {
          button.setDisabled(true);
          button.setButtonText("Publishing...");

          await this.plugin.fullSync();

          button.setDisabled(false);
          button.setButtonText("Start publish");
        }),
      );

    new Setting(containerEl)
      .setName("Auto publish")
      .setDesc("Automatically publish on file changes")
      .addToggle((toggle) =>
        toggle
          .setValue(this.plugin.settings.autoSync)
          .onChange(async (value) => {
            this.plugin.settings.autoSync = value;
            await this.plugin.saveSettings();
          }),
      );
  }
}
