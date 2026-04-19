"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useCreateMode, useUpdateMode } from "@/lib/hooks";
import type { ModelMode } from "@/lib/hooks/use-model-families";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogFooter,
  Button,
  Input,
  Label,
  Select,
  SelectOption,
  Switch,
} from "@/components/ui";
import { useToast } from "@/components/ui/toaster";
import { useTranslation } from "@/lib/i18n/context";
import { getAPIErrorMessage } from "@/lib/api-client";

const modeSchema = z.object({
  mode: z.string().min(1, "Sub-suffix is required"),
  upstream_mode: z.string(),
  enabled: z.boolean(),
  pool_floor_override: z.string().optional(),
  force_thinking: z.boolean(),
  enable_pro: z.boolean(),
});

type ModeInput = z.infer<typeof modeSchema>;

interface ModeDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  familyId: number;
  familyType: string;
  mode?: ModelMode;
}

const defaults: ModeInput = {
  mode: "",
  upstream_mode: "",
  enabled: true,
  pool_floor_override: "",
  force_thinking: false,
  enable_pro: false,
};

export function ModeDialog({
  open,
  onOpenChange,
  familyId,
  familyType,
  mode,
}: ModeDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const createMode = useCreateMode();
  const updateMode = useUpdateMode();
  const isEdit = !!mode;
  const isDefaultMode = isEdit && mode?.mode === "default";
  const requiresUpstream = familyType !== "image_ws" && familyType !== "image";
  const showEnablePro = familyType === "image_ws" || familyType === "image";

  const form = useForm<ModeInput>({
    resolver: zodResolver(modeSchema),
    defaultValues: defaults,
  });

  useEffect(() => {
    if (!open) {
      form.reset(defaults);
      return;
    }
    if (mode) {
      form.reset({
        mode: mode.mode,
        upstream_mode: mode.upstream_mode,
        enabled: mode.enabled,
        pool_floor_override: mode.pool_floor_override || "",
        force_thinking: mode.force_thinking ?? false,
        enable_pro: mode.enable_pro ?? false,
      });
    } else {
      form.reset(defaults);
    }
  }, [open, mode, form]);

  const onSubmit = async (data: ModeInput) => {
    const upstreamMode = data.upstream_mode.trim();
    if (requiresUpstream && !upstreamMode) {
      form.setError("upstream_mode", {
        type: "manual",
        message: "Upstream mode is required",
      });
      return;
    }
    try {
      const payload = {
        ...data,
        model_id: familyId,
        pool_floor_override: data.pool_floor_override || null,
        upstream_mode: requiresUpstream ? upstreamMode : "",
      };
      if (isEdit && mode) {
        await updateMode.mutateAsync({ id: mode.id, data: payload });
        toast({ title: t.common.success, description: t.models.updateSuccess });
      } else {
        await createMode.mutateAsync({ ...payload, model_id: familyId });
        toast({ title: t.common.success, description: t.models.createSuccess });
      }
      onOpenChange(false);
    } catch (error) {
      toast({
        title: t.common.error,
        description: getAPIErrorMessage(error, t.common.operationFailed),
        variant: "destructive",
      });
    }
  };

  const isPending = createMode.isPending || updateMode.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t.models.editMode : t.models.createMode}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="mode">{t.models.subSuffix} *</Label>
            <Input
              id="mode"
              {...form.register("mode")}
              placeholder="fast"
              readOnly={isDefaultMode}
              className={isDefaultMode ? "bg-muted" : ""}
            />
            {isDefaultMode && (
              <p className="text-sm text-muted-foreground">
                {t.models.defaultModeReadonly}
              </p>
            )}
            {form.formState.errors.mode && (
              <p className="text-sm text-destructive">
                {form.formState.errors.mode.message}
              </p>
            )}
          </div>
          {requiresUpstream ? (
            <div className="space-y-2">
              <Label htmlFor="upstream_mode">{t.models.upstreamMode} *</Label>
              <Select id="upstream_mode" {...form.register("upstream_mode")}>
                <SelectOption value="">—</SelectOption>
                <SelectOption value="auto">auto</SelectOption>
                <SelectOption value="fast">fast</SelectOption>
                <SelectOption value="expert">expert</SelectOption>
                <SelectOption value="heavy">heavy</SelectOption>
              </Select>
              {form.formState.errors.upstream_mode && (
                <p className="text-sm text-destructive">
                  {form.formState.errors.upstream_mode.message}
                </p>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted">
              {t.models.imageNoUpstream}
            </p>
          )}
          <div className="space-y-2">
            <Label htmlFor="pool_floor_override">
              {t.models.poolFloorOverride}
            </Label>
            <Select
              id="pool_floor_override"
              {...form.register("pool_floor_override")}
            >
              <SelectOption value="">—</SelectOption>
              <SelectOption value="basic">Basic</SelectOption>
              <SelectOption value="super">Super</SelectOption>
              <SelectOption value="heavy">Heavy</SelectOption>
            </Select>
          </div>
          {requiresUpstream && (
            <div className="flex items-center justify-between">
              <div>
                <Label>{t.models.forceThinking}</Label>
                <p className="text-sm text-muted-foreground">
                  {t.models.forceThinkingDesc}
                </p>
              </div>
              <Switch
                checked={form.watch("force_thinking")}
                onCheckedChange={(v) =>
                  form.setValue("force_thinking", v, { shouldDirty: true })
                }
              />
            </div>
          )}
          {showEnablePro && (
            <div className="flex items-center justify-between">
              <div>
                <Label>{t.models.enablePro}</Label>
                <p className="text-sm text-muted-foreground">
                  {t.models.enableProDesc}
                </p>
              </div>
              <Switch
                checked={form.watch("enable_pro")}
                onCheckedChange={(v) =>
                  form.setValue("enable_pro", v, { shouldDirty: true })
                }
              />
            </div>
          )}
          <div className="flex items-center justify-between pt-1">
            <Label htmlFor="enabled">{t.common.enabled}</Label>
            <Switch
              id="enabled"
              checked={form.watch("enabled")}
              onCheckedChange={(v) =>
                form.setValue("enabled", v, { shouldDirty: true })
              }
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t.common.cancel}
            </Button>
            <Button type="submit" disabled={isPending}>
              {isEdit ? t.models.saveMode : t.models.createMode}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
