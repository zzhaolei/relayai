#import <Cocoa/Cocoa.h>
#include "appearance.h"

void setWindowAppearanceMode(int mode) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NSAppearance *appearance = nil;
        if (mode == 1) {
            appearance = [NSAppearance appearanceNamed:NSAppearanceNameDarkAqua];
        } else if (mode == 0) {
            appearance = [NSAppearance appearanceNamed:NSAppearanceNameAqua];
        }

        for (NSWindow *window in [NSApp windows]) {
            [window setAppearance:appearance];
        }
    });
}
