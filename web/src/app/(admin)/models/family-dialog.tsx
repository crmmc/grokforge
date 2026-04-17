"use client";

import { useEffect } from "react";
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";
import { useCreateFamily, useUpdateFamily } from "@/lib/hooks";
import type { FamilyWithModes } from "@/lib/hooks/use-model-families";
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

const familySchema = z.object({
  model: z.string().min(1, "Model name is required"),
  display_name: z.string().optional(),
  type: z.enum(["chat", "image", "image_edit", "video"]),
  pool_floor: z.enum(["basic", "super", "heavy"]),
  default_mode_id: z.number().int().positive().nullable(),
  quota_default: z.string().optional(),
  enabled: z.boolean(),
  description: z.string().optional(),
});

type FamilyInput = z.infer<typeof familySchema>;

interface FamilyDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  family?: FamilyWithModes;
}

const defaults: FamilyInput = {
  model: "",
  display_name: "",
  type: "chat",
  pool_floor: "basic",
  default_mode_id: null,
  quota_default: "",
  enabled: true,
  description: "",
};

export function FamilyDialog({
  open,
  onOpenChange,
  family,
}: FamilyDialogProps) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const createFamily = useCreateFamily();
  const updateFamily = useUpdateFamily();
  const isEdit = !!family;

  const form = useForm<FamilyInput>({
    resolver: zodResolver(familySchema),
    defaultValues: defaults,
  });

  useEffect(() => {
    if (!open) {
      form.reset(defaults);
      return;
    }
    if (family) {
      form.reset({
        model: family.model,
        display_name: family.display_name || "",
        type: family.type as FamilyInput["type"],
        pool_floor: family.pool_floor as FamilyInput["pool_floor"],
        default_mode_id: family.default_mode_id,
        quota_default: family.quota_default || "",
        enabled: family.enabled,
        description: family.description || "",
      });
    } else {
      form.reset(defaults);
    }
  }, [open, family, form]);

  const onSubmit = async (data: FamilyInput) => {
    try {
      const { default_mode_id, ...rest } = data;
      const base = {
        ...rest,
        quota_default: data.quota_default?.trim()
          ? data.quota_default.trim()
          : null,
      };
      if (isEdit && family) {
        await updateFamily.mutateAsync({
          id: family.id,
          data: { ...base, default_mode_id },
        });
        toast({ title: t.common.success, description: t.models.updateSuccess });
      } else {
        await createFamily.mutateAsync(base);
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

  const isPending = createFamily.isPending || updateFamily.isPending;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[480px]">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? t.models.editFamily : t.models.createFamily}
          </DialogTitle>
        </DialogHeader>
        <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="model">Model *</Label>
            <Input
              id="model"
              {...form.register("model")}
              placeholder="grok-4.20"
            />
            {form.formState.errors.model && (
              <p className="text-sm text-destructive">
                {form.formState.errors.model.message}
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="display_name">Display Name</Label>
            <Input
              id="display_name"
              {...form.register("display_name")}
              placeholder="Grok 4.20"
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="type">Type</Label>
              <Select id="type" {...form.register("type")}>
                <SelectOption value="chat">Chat</SelectOption>
                <SelectOption value="image">Image</SelectOption>
                <SelectOption value="image_edit">Image Edit</SelectOption>
                <SelectOption value="video">Video</SelectOption>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="pool_floor">Pool Floor</Label>
              <Select id="pool_floor" {...form.register("pool_floor")}>
                <SelectOption value="basic">Basic</SelectOption>
                <SelectOption value="super">Super</SelectOption>
                <SelectOption value="heavy">Heavy</SelectOption>
              </Select>
            </div>
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-2">
              <Label htmlFor="default_mode_id">Default Mode</Label>
              <Select
                id="default_mode_id"
                value={
                  form.watch("default_mode_id") != null
                    ? String(form.watch("default_mode_id"))
                    : ""
                }
                onChange={(e) =>
                  form.setValue(
                    "default_mode_id",
                    e.target.value ? Number(e.target.value) : null,
                    { shouldDirty: true },
                  )
                }
                disabled={!family || family.modes.length === 0}
              >
                <SelectOption value="">—</SelectOption>
                {family?.modes.map((mode) => (
                  <SelectOption key={mode.id} value={String(mode.id)}>
                    {mode.mode}
                  </SelectOption>
                ))}
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="quota_default">Quota Default</Label>
              <Input
                id="quota_default"
                {...form.register("quota_default")}
                placeholder='{"chat": 100}'
              />
            </div>
          </div>
          <div className="space-y-2">
            <Label htmlFor="description">Description</Label>
            <Input id="description" {...form.register("description")} />
          </div>
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
              {isEdit ? t.models.saveFamily : t.models.createFamily}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
