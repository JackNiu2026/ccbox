using System.ComponentModel;

namespace Cage.Windows;

internal sealed class TrayMenu : ContextMenuStrip
{
    public TrayMenu()
    {
        ShowImageMargin = false;
        ShowCheckMargin = false;
        DropShadowEnabled = true;
        Font = new Font(SystemFonts.MenuFont!.FontFamily, 9.5f, FontStyle.Regular, GraphicsUnit.Point);
    }

    protected override void OnOpening(CancelEventArgs e)
    {
        base.OnOpening(e);
        Padding = new Padding(LogicalToDeviceUnits(6));
        int width = LogicalToDeviceUnits(156);
        foreach (ToolStripItem item in Items)
        {
            item.Padding = new Padding(LogicalToDeviceUnits(12), LogicalToDeviceUnits(7),
                LogicalToDeviceUnits(12), LogicalToDeviceUnits(7));
            width = Math.Max(width, item.GetPreferredSize(Size.Empty).Width + Padding.Horizontal);
        }
        foreach (ToolStripItem item in Items)
        {
            item.AutoSize = false;
            int height = Math.Max(LogicalToDeviceUnits(34), item.GetPreferredSize(Size.Empty).Height);
            item.Size = new Size(width - Padding.Horizontal, height);
        }
        MinimumSize = new Size(width, 0);
    }
}
